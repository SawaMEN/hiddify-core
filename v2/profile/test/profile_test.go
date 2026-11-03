package test

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hiddify/hiddify-core/v2/profile"
	"github.com/sagernet/sing-box/experimental/libbox"
)

func TestAddByContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Profile-Title", "base64:"+base64.StdEncoding.EncodeToString([]byte("🔥 WARP 🔥")))
		w.Header().Set("Subscription-Userinfo", "upload=0; download=0; total=10737418240000000; expire=2546249531")
		w.Header().Set("Support-Url", "https://t.me/hiddify")
		w.Header().Set("Profile-Web-Page-Url", "https://hiddify.com")
		fmt.Fprintln(w, `{"outbounds":[{"type":"socks","tag":"test-proxy","server":"127.0.0.1","server_port":1080}]}`)
	}))
	defer server.Close()
	ctx := libbox.BaseContext(nil)
	entity, err := profile.AddByUrl(ctx, server.URL, "", false)
	if err != nil {
		t.Fatalf("expected no error, but got: %v", err)
	}
	fmt.Printf("entity: %v\n", entity)
	// Check if the content has been added correctly
	profileTitle := entity.Name
	expectedTitle := "🔥 WARP 🔥" // The Base64 decoded title
	if profileTitle != expectedTitle {
		t.Errorf("expected profile title to be %v, got %v", expectedTitle, profileTitle)
	}

	// Check subscription userinfo
	userInfo := entity.SubInfo
	if userInfo.Upload != 0 || userInfo.Download != 0 || userInfo.Total != 10737418240000000 || userInfo.Expire != 2546249531 {
		t.Errorf("subscription userinfo not parsed correctly, got: %v", userInfo)
	}

	// Check URLs
	supportURL := entity.SubInfo.SupportUrl
	if supportURL != "https://t.me/hiddify" {
		t.Errorf("expected support URL to be https://t.me/hiddify, got %v", supportURL)
	}

	profileWebPageURL := entity.SubInfo.WebPageUrl
	if profileWebPageURL != "https://hiddify.com" {
		t.Errorf("expected profile web page URL to be https://hiddify.com, got %v", profileWebPageURL)
	}
	profile.DeleteById(entity.Id)
	// You can further assert individual fields of warp configurations
}
