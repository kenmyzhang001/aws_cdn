package cloudflare

import (
	"testing"
)

// 与 pagesFileHash/pag​​esAssetContentHash 当前实现保持一致：
// SHA-256(base64(content) + "html") 的前 32 个 hex。
func TestPagesAssetContentHash_indexHTML(t *testing.T) {
	content := []byte("<!doctype html><html></html>")
	got := pagesAssetContentHash(content, "/index.html")
	want := "5757068450baa3236520edac82293a8f"
	if got != want {
		t.Fatalf("got %q want %q (若失败说明与 Wrangler 算法不一致)", got, want)
	}
}
