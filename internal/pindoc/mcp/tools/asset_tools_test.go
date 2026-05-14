package tools

import (
	"context"
	"encoding/base64"
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

func TestDecodeAssetUploadInputLocalPathLoopbackOnly(t *testing.T) {
	tmp := t.TempDir()
	localPath := tmp + "/asset-note.txt"
	if err := os.WriteFile(localPath, []byte("loopback asset"), 0o644); err != nil {
		t.Fatalf("write temp asset: %v", err)
	}

	oauth := &auth.Principal{UserID: "00000000-0000-0000-0000-000000000001", AgentID: "agent:oauth", Source: auth.SourceOAuth}
	content, filename, out := decodeAssetUploadInput(assetUploadInput{LocalPath: localPath}, oauth)
	if out.Status != "not_ready" || out.ErrorCode != "ASSET_LOCAL_PATH_LOOPBACK_ONLY" {
		t.Fatalf("oauth local_path output = %+v", out)
	}
	if content != nil || filename != "" {
		t.Fatalf("oauth local_path content=%q filename=%q, want empty", string(content), filename)
	}

	loopback := &auth.Principal{AgentID: "agent:asset-test", Source: auth.SourceLoopback}
	content, filename, out = decodeAssetUploadInput(assetUploadInput{LocalPath: localPath}, loopback)
	if out.Status != "" || string(content) != "loopback asset" || filename != "asset-note.txt" {
		t.Fatalf("loopback local_path content=%q filename=%q output=%+v", string(content), filename, out)
	}

	content, filename, out = decodeAssetUploadInput(assetUploadInput{
		BytesBase64: base64.StdEncoding.EncodeToString([]byte("oauth bytes")),
		Filename:    "oauth.txt",
	}, oauth)
	if out.Status != "" || string(content) != "oauth bytes" || filename != "oauth.txt" {
		t.Fatalf("oauth bytes content=%q filename=%q output=%+v", string(content), filename, out)
	}
}

func TestDecodeAssetUploadInputWindowsHostPathFailureGuidance(t *testing.T) {
	loopback := &auth.Principal{AgentID: "agent:asset-test", Source: auth.SourceLoopback}
	content, filename, out := decodeAssetUploadInput(assetUploadInput{LocalPath: `A:\pindoc\missing-image.png`}, loopback)
	if out.Status != "not_ready" || out.ErrorCode != "ASSET_LOCAL_READ_FAILED" {
		t.Fatalf("windows host local_path output = %+v", out)
	}
	if content != nil || filename != "" {
		t.Fatalf("windows host local_path content=%q filename=%q, want empty", string(content), filename)
	}
	if len(out.ErrorCodes) == 0 || out.ErrorCodes[0] != "ASSET_LOCAL_READ_FAILED" {
		t.Fatalf("windows host local_path error_codes = %v", out.ErrorCodes)
	}
	if len(out.ChecklistItems) == 0 || out.ChecklistItems[0].Code != "ASSET_LOCAL_READ_FAILED" {
		t.Fatalf("windows host local_path checklist_items = %+v", out.ChecklistItems)
	}
	actions := strings.Join(out.SuggestedActions, "\n")
	for _, want := range []string{"tools/push-asset.ps1", "/tmp/pindoc-asset-upload", "bytes_base64", "Docker Desktop"} {
		if !strings.Contains(actions, want) {
			t.Fatalf("windows host local_path suggested_actions missing %q: %v", want, out.SuggestedActions)
		}
	}
}

func TestAssetToolDescriptionsClarifyDockerAndInlineImages(t *testing.T) {
	for _, want := range []string{"Docker Desktop on Windows", "tools/push-asset.ps1", "docker cp", "/tmp/pindoc-asset-upload", "asset.blob_url"} {
		if !strings.Contains(assetUploadToolDescription, want) {
			t.Fatalf("asset upload description missing %q: %q", want, assetUploadToolDescription)
		}
	}
	for _, want := range []string{"metadata only", "does not insert image Markdown", "body_markdown", "![alt](asset.blob_url)", "role=inline_image"} {
		if !strings.Contains(assetAttachToolDescription, want) {
			t.Fatalf("asset attach description missing %q: %q", want, assetAttachToolDescription)
		}
	}
}

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

func TestAssetUploadLocalPathLoopbackOnlyIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run asset local_path trust-boundary integration")
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
	projectSlug := "asset-local-path-" + strings.ReplaceAll(time.Unix(0, suffix).Format("150405.000000000"), ".", "-")
	projectID := insertContextReceiptProject(t, ctx, pool, projectSlug)
	userID := insertMCPVisibilityUser(t, ctx, pool, "asset-local-path-"+projectSlug+"@example.invalid")
	if _, err := pool.Exec(ctx, `
		INSERT INTO project_members (project_id, user_id, role)
		VALUES ($1::uuid, $2::uuid, 'editor')
	`, projectID, userID); err != nil {
		t.Fatalf("insert oauth project member: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id = $1::uuid`, projectID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1::uuid`, userID)
	})

	tmp := t.TempDir()
	localPath := tmp + "/host-file.txt"
	if err := os.WriteFile(localPath, []byte("host-only content"), 0o644); err != nil {
		t.Fatalf("write local path asset: %v", err)
	}

	oauthPrincipal := &auth.Principal{UserID: userID, AgentID: "agent:oauth-asset", Source: auth.SourceOAuth}
	oauthCall := newAssetToolTestCallerWithPrincipal(t, ctx, pool, tmp, oauthPrincipal)
	rejected := oauthCall(ctx, "pindoc.asset.upload", map[string]any{
		"project_slug": projectSlug,
		"local_path":   localPath,
	})
	if rejected.Status != "not_ready" || rejected.ErrorCode != "ASSET_LOCAL_PATH_LOOPBACK_ONLY" {
		t.Fatalf("oauth local_path output = %+v", rejected)
	}
	var assetCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM assets WHERE project_id = $1::uuid`, projectID).Scan(&assetCount); err != nil {
		t.Fatalf("count assets after rejected local_path: %v", err)
	}
	if assetCount != 0 {
		t.Fatalf("assets after rejected local_path = %d, want 0", assetCount)
	}

	loopbackCall := newAssetToolTestCallerWithPrincipal(t, ctx, pool, tmp, &auth.Principal{AgentID: "agent:asset-test", Source: auth.SourceLoopback})
	loopback := loopbackCall(ctx, "pindoc.asset.upload", map[string]any{
		"project_slug": projectSlug,
		"local_path":   localPath,
	})
	if loopback.Status != "accepted" || loopback.Asset == nil || loopback.AssetRef == "" {
		t.Fatalf("loopback local_path output = %+v", loopback)
	}

	oauthBytes := oauthCall(ctx, "pindoc.asset.upload", map[string]any{
		"project_slug": projectSlug,
		"bytes_base64": base64.StdEncoding.EncodeToString([]byte("oauth bytes content")),
		"filename":     "oauth-bytes.txt",
		"mime_type":    "text/plain",
	})
	if oauthBytes.Status != "accepted" || oauthBytes.Asset == nil || oauthBytes.AssetRef == "" {
		t.Fatalf("oauth bytes output = %+v", oauthBytes)
	}
}

func newAssetToolTestCaller(t *testing.T, ctx context.Context, pool *db.Pool, assetRoot string) func(context.Context, string, map[string]any) assetToolOutput {
	t.Helper()
	return newAssetToolTestCallerWithPrincipal(t, ctx, pool, assetRoot, &auth.Principal{
		AgentID: "agent:asset-test",
		Source:  auth.SourceLoopback,
	})
}

func newAssetToolTestCallerWithPrincipal(t *testing.T, ctx context.Context, pool *db.Pool, assetRoot string, principal *auth.Principal) func(context.Context, string, map[string]any) assetToolOutput {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := sdk.NewServer(&sdk.Implementation{Name: "pindoc-asset-test", Version: "test"}, nil)
	deps := Deps{
		DB:        pool,
		Logger:    logger,
		AuthChain: auth.NewChain(staticAssetToolResolver{p: principal}),
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

type staticAssetToolResolver struct {
	p *auth.Principal
}

func (r staticAssetToolResolver) Resolve(context.Context, *sdk.CallToolRequest) (*auth.Principal, error) {
	return r.p, nil
}
