package mcp

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/mcp/tools"
)

func TestShouldNotifyToolsetListChanged(t *testing.T) {
	tests := []struct {
		name        string
		hadPrevious bool
		previous    string
		current     string
		want        bool
	}{
		{name: "no previous is first boot", current: "29:new", want: false},
		{name: "same version is quiet", hadPrevious: true, previous: "29:same", current: "29:same", want: false},
		{name: "changed version notifies", hadPrevious: true, previous: "28:old", current: "29:new", want: true},
		{name: "empty current is quiet", hadPrevious: true, previous: "28:old", current: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldNotifyToolsetListChanged(tt.hadPrevious, tt.previous, tt.current); got != tt.want {
				t.Fatalf("shouldNotifyToolsetListChanged() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToolsetListChangedNotifierInMemoryTransport(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	sdkServer := newReannounceSDKServer(t, "pindoc.test.reannounce.inmemory")
	pindocServer := newChangedToolsetServer(sdkServer)
	pindocServer.StartToolsetListChangedNotifier(ctx)

	clientTransport, serverTransport := sdk.NewInMemoryTransports()
	serverSession, err := sdkServer.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect() error = %v", err)
	}
	defer serverSession.Close()

	notifications := make(chan struct{}, 1)
	client := sdk.NewClient(&sdk.Implementation{Name: "toolset-notify-test-client"}, &sdk.ClientOptions{
		ToolListChangedHandler: func(context.Context, *sdk.ToolListChangedRequest) {
			notifications <- struct{}{}
		},
	})
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}
	defer clientSession.Close()

	expectToolListChanged(t, ctx, notifications)
}

func TestToolsetListChangedNotifierStreamableHTTP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	sdkServer := newReannounceSDKServer(t, "pindoc.test.reannounce.streamable")
	pindocServer := newChangedToolsetServer(sdkServer)
	pindocServer.StartToolsetListChangedNotifier(ctx)

	httpServer := httptest.NewServer(sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server {
		return sdkServer
	}, nil))
	defer httpServer.Close()

	notifications := make(chan struct{}, 1)
	client := sdk.NewClient(&sdk.Implementation{Name: "toolset-notify-http-client"}, &sdk.ClientOptions{
		ToolListChangedHandler: func(context.Context, *sdk.ToolListChangedRequest) {
			notifications <- struct{}{}
		},
	})
	clientSession, err := client.Connect(ctx, &sdk.StreamableClientTransport{Endpoint: httpServer.URL}, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}
	defer clientSession.Close()

	expectToolListChanged(t, ctx, notifications)
}

func TestToolsetListChangedNotifierQuietWhenVersionUnchanged(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	sdkServer := newReannounceSDKServer(t, "pindoc.test.reannounce.quiet")
	pindocServer := &Server{
		sdk:    sdkServer,
		logger: testLogger(),
		toolsetListChanged: &toolsetListChangedNotifier{notice: toolsetChangeNotice{
			Current:  "29:same",
			Previous: "29:same",
			Changed:  false,
		}},
	}
	pindocServer.StartToolsetListChangedNotifier(ctx)

	clientTransport, serverTransport := sdk.NewInMemoryTransports()
	serverSession, err := sdkServer.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect() error = %v", err)
	}
	defer serverSession.Close()
	notifications := make(chan struct{}, 1)
	client := sdk.NewClient(&sdk.Implementation{Name: "toolset-notify-quiet-client"}, &sdk.ClientOptions{
		ToolListChangedHandler: func(context.Context, *sdk.ToolListChangedRequest) {
			notifications <- struct{}{}
		},
	})
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}
	defer clientSession.Close()

	select {
	case <-notifications:
		t.Fatalf("unexpected tools/list_changed notification for unchanged toolset")
	case <-ctx.Done():
	}
}

func newReannounceSDKServer(t *testing.T, toolName string) *sdk.Server {
	t.Helper()
	server := sdk.NewServer(&sdk.Implementation{Name: "pindoc-test", Version: "test"}, nil)
	deps := tools.Deps{
		AuthChain:    auth.NewChain(auth.NewTrustedLocalResolver("", "agent:test")),
		UserLanguage: "en",
	}
	tools.AddInstrumentedTool[struct{}, struct{}](server, deps,
		&sdk.Tool{Name: toolName, Description: "test reannounce trigger"},
		func(context.Context, *auth.Principal, struct{}) (*sdk.CallToolResult, struct{}, error) {
			return nil, struct{}{}, nil
		},
	)
	return server
}

func newChangedToolsetServer(sdkServer *sdk.Server) *Server {
	return &Server{
		sdk:    sdkServer,
		logger: testLogger(),
		toolsetListChanged: &toolsetListChangedNotifier{notice: toolsetChangeNotice{
			Current:  "29:new",
			Previous: "28:old",
			Changed:  true,
		}},
	}
}

func expectToolListChanged(t *testing.T, ctx context.Context, notifications <-chan struct{}) {
	t.Helper()
	select {
	case <-notifications:
	case <-ctx.Done():
		t.Fatalf("did not receive tools/list_changed notification: %v", ctx.Err())
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
