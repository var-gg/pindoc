package tools

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/db"
)

func TestAssetToolsUploadReadAttachIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run asset MCP DB integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool.Pool); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	suffix := time.Now().UnixNano()
	projectSlug := "asset-tools-" + strings.ReplaceAll(time.Unix(0, suffix).Format("150405.000000000"), ".", "-")
	projectID := insertContextReceiptProject(t, ctx, pool, projectSlug)
	areaID := insertContextReceiptArea(t, ctx, pool, projectID, "mcp")
	artifactID := insertDryRunArtifact(t, ctx, pool, projectID, areaID, "asset-target", "Decision", "Asset target", validDecisionBodyForPropose("asset context", "asset decision"))
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id = $1::uuid`, projectID)
	}()

	tmp := t.TempDir()
	localPath := tmp + "/asset-note.txt"
	if err := os.WriteFile(localPath, []byte("hello asset"), 0o644); err != nil {
		t.Fatalf("write temp asset: %v", err)
	}
	call := newAssetToolTestCaller(t, ctx, pool, tmp)

	uploaded := call(ctx, "pindoc.asset.upload", map[string]any{
		"project_slug": projectSlug,
		"local_path":   localPath,
	})
	if uploaded.Status != "accepted" || uploaded.Asset == nil || uploaded.AssetRef == "" {
		t.Fatalf("upload output = %+v", uploaded)
	}
	if uploaded.Asset.MimeType != "text/plain" || uploaded.Asset.BlobURL == "" {
		t.Fatalf("uploaded asset metadata = %+v", uploaded.Asset)
	}
	if strings.Contains(uploaded.Asset.BlobURL, tmp) {
		t.Fatalf("blob_url leaked local path: %q", uploaded.Asset.BlobURL)
	}

	read := call(ctx, "pindoc.asset.read", map[string]any{
		"project_slug": projectSlug,
		"asset_id":     uploaded.AssetRef,
	})
	if read.Status != "accepted" || read.Asset == nil || read.Asset.ID != uploaded.Asset.ID {
		t.Fatalf("read output = %+v", read)
	}

	attached := call(ctx, "pindoc.asset.attach", map[string]any{
		"project_slug": projectSlug,
		"asset_id":     uploaded.Asset.ID,
		"artifact":     "asset-target",
		"role":         "attachment",
	})
	if attached.Status != "accepted" || attached.Attachment == nil {
		t.Fatalf("attach output = %+v", attached)
	}
	if attached.Attachment.ArtifactID != artifactID || attached.Attachment.RevisionNumber != 1 {
		t.Fatalf("attachment = %+v, artifactID=%s", attached.Attachment, artifactID)
	}
	var relationCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		  FROM artifact_assets
		 WHERE artifact_id = $1::uuid AND asset_id = $2::uuid AND role = 'attachment'
	`, artifactID, uploaded.Asset.ID).Scan(&relationCount); err != nil {
		t.Fatalf("count artifact asset relations: %v", err)
	}
	if relationCount != 1 {
		t.Fatalf("relation count = %d, want 1", relationCount)
	}

	publicArtifactID := insertDryRunArtifact(t, ctx, pool, projectID, areaID, "asset-public-target", "Decision", "Asset public target", validDecisionBodyForPropose("asset public context", "asset public decision"))
	if _, err := pool.Exec(ctx, `UPDATE artifacts SET visibility = 'public' WHERE id = $1::uuid`, publicArtifactID); err != nil {
		t.Fatalf("mark public artifact: %v", err)
	}
	publicAttach := call(ctx, "pindoc.asset.attach", map[string]any{
		"project_slug": projectSlug,
		"asset_id":     uploaded.Asset.ID,
		"artifact":     "asset-public-target",
		"role":         "attachment",
	})
	if publicAttach.Status != "accepted" || len(publicAttach.Warnings) == 0 || !strings.Contains(publicAttach.Warnings[0], "ASSET_SHARED_PUBLIC") {
		t.Fatalf("public attach warnings = %+v", publicAttach.Warnings)
	}
	privateAttach := call(ctx, "pindoc.asset.attach", map[string]any{
		"project_slug": projectSlug,
		"asset_id":     uploaded.Asset.ID,
		"artifact":     "asset-target",
		"role":         "evidence",
	})
	if privateAttach.Status != "accepted" || len(privateAttach.Warnings) == 0 || !strings.Contains(privateAttach.Warnings[0], "ASSET_ALREADY_PUBLIC") {
		t.Fatalf("private attach warnings = %+v", privateAttach.Warnings)
	}

	rejected := call(ctx, "pindoc.asset.upload", map[string]any{
		"project_slug": projectSlug,
		"bytes_base64": "AAECAwQ=",
		"filename":     "blob.bin",
		"mime_type":    "application/octet-stream",
	})
	if rejected.Status != "not_ready" || rejected.ErrorCode != "ASSET_MIME_UNSUPPORTED" {
		t.Fatalf("unsupported upload = %+v", rejected)
	}
}

func newAssetToolTestCaller(t *testing.T, ctx context.Context, pool *db.Pool, assetRoot string) func(context.Context, string, map[string]any) assetToolOutput {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := sdk.NewServer(&sdk.Implementation{Name: "pindoc-asset-test", Version: "test"}, nil)
	deps := Deps{
		DB:        pool,
		Logger:    logger,
		AuthChain: auth.NewChain(auth.NewTrustedLocalResolver("", "agent:asset-test")),
		AssetRoot: assetRoot,
	}
	RegisterAssetUpload(server, deps)
	RegisterAssetRead(server, deps)
	RegisterAssetAttach(server, deps)
	clientTransport, serverTransport := sdk.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "asset-test-client"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		clientSession.Close()
		serverSession.Wait()
	})
	return func(callCtx context.Context, name string, args map[string]any) assetToolOutput {
		t.Helper()
		res, err := clientSession.CallTool(callCtx, &sdk.CallToolParams{
			Name:      name,
			Arguments: args,
		})
		if err != nil {
			t.Fatalf("CallTool %s: %v", name, err)
		}
		if res.IsError {
			t.Fatalf("%s result error: %s", name, toolResultText(res))
		}
		var out assetToolOutput
		if err := decodeStructuredContent(res.StructuredContent, &out); err != nil {
			t.Fatalf("decode %s structured content: %v", name, err)
		}
		return out
	}
}
