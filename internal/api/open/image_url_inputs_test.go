package open

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/mirainya/Prism/pkg/config"
)

func TestImageURLInputsPreserveAddressesWithoutStorage(t *testing.T) {
	previous := config.C
	config.C = nil
	t.Cleanup(func() { config.C = previous })
	urls := []string{"https://8.8.8.8/first.png?signature=original", "https://1.1.1.1/second.webp"}
	inputs, err := importOpenAIImageValues(context.Background(), 41, 7, urls)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(inputs.URLs, urls) || len(inputs.AssetIDs) != 0 {
		t.Fatalf("inputs=%+v", inputs)
	}
}

func TestMixedImageInputsKeepURLAndBase64Order(t *testing.T) {
	image := testOpenAIImagePNG()
	first, last := "https://8.8.8.8/first.png", "https://1.1.1.1/last.png"
	prepared, err := prepareOpenAIImageValues(context.Background(), []string{first, "data:image/png;base64," + base64.StdEncoding.EncodeToString(image), last})
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared) != 3 || prepared[0].URL != first || len(prepared[0].Data) != 0 ||
		prepared[1].URL != "" || string(prepared[1].Data) != string(image) || prepared[1].ContentType != "image/png" ||
		prepared[2].URL != last || len(prepared[2].Data) != 0 {
		t.Fatalf("mixed inputs changed: %+v", prepared)
	}
}

func TestImageEditURLReachesRoutingWithoutConfiguredStorage(t *testing.T) {
	for _, async := range []bool{false, true} {
		t.Run(map[bool]string{false: "sync", true: "async"}[async], func(t *testing.T) {
			previous := unifiedVideoStore
			unifiedVideoStore = nil
			t.Cleanup(func() { unifiedVideoStore = previous })
			body, _ := json.Marshal(map[string]any{"model": "image-model", "prompt": "edit", "image_urls": []string{"https://8.8.8.8/input.png"}})
			handler := CreateImageEditOpenAI
			if async {
				handler = CreateImageEditAsync
			}
			response := performOpenAIImageRequest(t, handler, "application/json", string(body))
			if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "store image") {
				t.Fatalf("URL blocked before routing: %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestImageEditRejectsUnsafeURLBeforeRouting(t *testing.T) {
	for _, location := range []string{"http://127.0.0.1/private", "http://10.0.0.1/private", "https://user:password@8.8.8.8/image.png", "https://8.8.8.8/image.png#fragment"} {
		body, _ := json.Marshal(map[string]any{"model": "image-model", "prompt": "edit", "image_urls": []string{location}})
		response := performOpenAIImageRequest(t, CreateImageEditAsync, "application/json", string(body))
		assertOpenAIImageBadRequest(t, response, "invalid image input URL")
	}
}
