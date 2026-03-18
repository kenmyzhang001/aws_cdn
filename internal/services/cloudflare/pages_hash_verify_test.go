package cloudflare

import (
	"testing"
)

// 与 Wrangler blake3-wasm（hash.ts）对齐：base64(内容)+"html" 的前 32 个 hex。
func TestPagesAssetContentHash_indexHTML(t *testing.T) {
	content := []byte("<!doctype html><html></html>")
	got := pagesAssetContentHash(content, "/index.html")
	want := "24c78875ef69415e546541d2f6346baa"
	if got != want {
		t.Fatalf("got %q want %q (若失败说明与 Wrangler 算法不一致)", got, want)
	}
}
