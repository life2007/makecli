/**
 * [INPUT]: 依赖 Client.ForApp 与资源 API、context、net/http、net/http/httptest、strings、testing
 * [OUTPUT]: 验证资源请求的 X-Make-App-Key、appRole 原值透传及客户端作用域隔离
 * [POS]: internal/api 的应用身份请求头契约测试，真实 HTTP 桩隔离网络
 * [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
 */
package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func appHeaderResult[T any](_ T, err error) error      { return err }
func appHeaderPage[T any](_ T, _ int, err error) error { return err }

func TestResourceAppKeyHeaders(t *testing.T) {
	const app = "shop_beta_"
	tests := []struct {
		name   string
		invoke func(*Client) error
	}{
		{"create app", func(c *Client) error { return c.CreateApp(app, "应用", nil) }},
		{"update app", func(c *Client) error { return c.UpdateApp(app, "应用", nil) }},
		{"delete app", func(c *Client) error { return c.DeleteApp(app) }},
		{"get app", func(c *Client) error { return appHeaderResult(c.GetApp(app)) }},
		{"deletion status", func(c *Client) error { return appHeaderResult(c.GetAppDeletionStatus(app)) }},
		{"create entity", func(c *Client) error { return c.CreateEntity("item", "项目", app, EntityProperties{}) }},
		{"update entity", func(c *Client) error { return c.UpdateEntity("item", "项目", app, EntityProperties{}) }},
		{"get entity", func(c *Client) error { return appHeaderResult(c.GetEntity(app, "item")) }},
		{"delete entity", func(c *Client) error { return c.DeleteEntity("item", app) }},
		{"list entities", func(c *Client) error { return appHeaderPage(c.ListEntities(app, 1, 10, "")) }},
		{"schema", func(c *Client) error { return appHeaderResult(c.GetSchema(app)) }},
		{"create relation", func(c *Client) error { return c.CreateRelation("rel", "关联", app, RelationProperties{}) }},
		{"update relation", func(c *Client) error { return c.UpdateRelation("rel", "关联", app, RelationProperties{}) }},
		{"get relation", func(c *Client) error { return appHeaderResult(c.GetRelation(app, "rel")) }},
		{"delete relation", func(c *Client) error { return c.DeleteRelation("rel", app) }},
		{"list relations", func(c *Client) error { return appHeaderPage(c.ListRelations(app, 1, 10, "")) }},
		{"create record", func(c *Client) error { return appHeaderResult(c.CreateRecord(app, "item", nil)) }},
		{"get record", func(c *Client) error { return appHeaderResult(c.GetRecord(app, "item", "1")) }},
		{"update record", func(c *Client) error { return c.UpdateRecord(app, "item", "1", nil) }},
		{"batch update records", func(c *Client) error { return c.UpdateRecordsBatch(app, "item", []string{"1"}, nil) }},
		{"delete records", func(c *Client) error { return appHeaderResult(c.DeleteRecords(app, "item", []string{"1"})) }},
		{"list records", func(c *Client) error { return appHeaderPage(c.ListRecords(app, "item", ListRecordOpts{})) }},
		{"aggregate records", func(c *Client) error { return appHeaderPage(c.AggregateRecords(app, "item", AggregateOpts{})) }},
		{"create variable", func(c *Client) error {
			return c.CreateVariable(Variable{Key: "URL", AppKey: app, Type: "Make.Variable"})
		}},
		{"update variable", func(c *Client) error {
			return c.UpdateVariable(Variable{Key: "URL", AppKey: app, Type: "Make.Variable"})
		}},
		{"get variable", func(c *Client) error { return appHeaderResult(c.GetVariable("URL", app, "runtime")) }},
		{"list variables", func(c *Client) error { return appHeaderPage(c.ListVariables(app, "runtime", 1, 10)) }},
		{"delete variable", func(c *Client) error { return c.DeleteVariable("URL", app, "runtime") }},
		{"write secret", func(c *Client) error {
			return c.CreateVariable(Variable{Key: "TOKEN", AppKey: app, Type: "Make.Secret"})
		}},
		{"get secret", func(c *Client) error { return appHeaderResult(c.GetSecret("TOKEN", app, "runtime")) }},
		{"list secrets", func(c *Client) error { return appHeaderPage(c.ListSecrets(app, "runtime", 1, 10)) }},
		{"delete secret", func(c *Client) error { return c.DeleteSecret("TOKEN", app, "runtime") }},
		{"create agent", func(c *Client) error {
			return appHeaderResult(c.CreateAgent(CreateAgentRequest{Key: "agent", AppKey: app}))
		}},
		{"get agent", func(c *Client) error { return appHeaderResult(c.GetAgent("agent", app)) }},
		{"delete agent", func(c *Client) error { return c.DeleteAgent("agent", app) }},
		{"agent status", func(c *Client) error { return appHeaderResult(c.SetAgentStatus("agent", app, "enabled")) }},
		{"promote", func(c *Client) error { return appHeaderResult(c.PromoteApp(app)) }},
		{"promote status", func(c *Client) error { return appHeaderResult(c.GetPromoteStatus(app, "promote_1")) }},
		{"repository", func(c *Client) error { return appHeaderResult(c.CreateRepository(app)) }},
		{"deployment overview", func(c *Client) error { return appHeaderResult(c.GetDeploymentOverview(app)) }},
		{"deployment logs", func(c *Client) error {
			return appHeaderResult(c.GetDeploymentLog(context.Background(), BuildTask{ID: 1, OrgID: 1, AppKey: app}, 0))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if got := r.Header.Get("X-Make-App-Key"); got != app {
					t.Errorf("X-Make-App-Key = %q, want %q", got, app)
				}
				if got := r.Header.Get("X-Make-Source"); got != "cli" {
					t.Errorf("X-Make-Source = %q, want cli", got)
				}
				_, _ = w.Write([]byte(`{"code":503,"msg":"test service unavailable"}`))
			}))
			defer srv.Close()
			client := New(srv.URL, "token", WithAppRole(RoleBeta)).ForApp("outer_app")
			if err := tt.invoke(client); err == nil {
				t.Fatal("expected service error")
			}
			if calls != 1 {
				t.Fatalf("requests = %d, want 1", calls)
			}
		})
	}
}

func TestForAppIsolation(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Header.Get("X-Make-App-Key"))
		_, _ = w.Write([]byte(`{"code":200,"data":[],"pagination":{"total":0}}`))
	}))
	defer srv.Close()
	base := New(srv.URL, "token")
	appA, appB := base.ForApp("app_a"), base.ForApp("app_b")
	clients := []*Client{appA, appB, base, appA, appA.ForApp("")}
	want := []string{"app_a", "app_b", "", "app_a", ""}
	for i, c := range clients {
		if _, _, err := c.ListApps(1, 10, ""); err != nil {
			t.Fatal(err)
		}
		if got[i] != want[i] {
			t.Errorf("request %d header = %q, want %q", i, got[i], want[i])
		}
	}
	if appA.httpClient != base.httpClient || appB.httpClient != base.httpClient {
		t.Fatal("scoped clients must share the connection pool")
	}
}

func TestScopedAppKeyHeaders(t *testing.T) {
	tests := []struct {
		name   string
		invoke func(*Client) error
	}{
		{"agent list", func(c *Client) error { return appHeaderPage(c.ListAgents(1, 10, "appKey == 'shop'")) }},
		{"build lookup", func(c *Client) error { return appHeaderResult(c.GetBuildTask("sha", EnvBeta)) }},
		{"build status", func(c *Client) error { return appHeaderResult(c.GetBuildTaskByIDContext(context.Background(), 1)) }},
		{"build log", func(c *Client) error { return appHeaderResult(c.GetBuildLog(context.Background(), 1, "")) }},
		{"OCR", func(c *Client) error { return appHeaderResult(c.OCR("x.pdf", strings.NewReader("pdf"), OCROptions{})) }},
	}
	for _, tt := range tests {
		for _, appKey := range []string{"", "shop"} {
			t.Run(tt.name+"/"+appKey, func(t *testing.T) {
				calls := 0
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					if got := r.Header.Get("X-Make-App-Key"); got != appKey {
						t.Errorf("X-Make-App-Key = %q, want %q", got, appKey)
					}
					if appKey == "" && len(r.Header.Values("X-Make-App-Key")) != 0 {
						t.Error("unscoped request must omit the header")
					}
					_, _ = w.Write([]byte(`{"code":503,"msg":"test service unavailable"}`))
				}))
				defer srv.Close()
				if err := tt.invoke(New(srv.URL, "token").ForApp(appKey)); err == nil {
					t.Fatal("expected service error")
				}
				if calls != 1 {
					t.Fatalf("requests = %d, want 1", calls)
				}
			})
		}
	}
}
