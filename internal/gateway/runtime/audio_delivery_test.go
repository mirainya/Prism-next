package runtime

import (
	"testing"

	"github.com/mirainya/Prism/internal/gateway/delivery"
)

func TestAudioManagedCopyPreservesFormatWithoutURLSuffix(t *testing.T) {
	for _, item := range []struct{ format, mime string }{
		{"wav", "audio/wav"}, {"mp3", "audio/mpeg"}, {"ogg", "audio/ogg"},
		{"opus", "audio/opus"}, {"flac", "audio/flac"}, {"m4a", "audio/mp4"}, {"aac", "audio/aac"}, {"pcm", "audio/pcm"},
	} {
		source := delivery.RemoteResult{Role: "audio", AudioFormat: item.format, URL: "https://assets.example/download?id=1"}
		recovered, err := delivery.DecodeManagedSource("image", delivery.EncodeManagedSource(source))
		if err != nil {
			t.Fatal(err)
		}
		if filename := managedResultImportFilename(recovered); filename != "result."+item.format {
			t.Fatalf("filename = %s", filename)
		}
		if filename := managedResultStorageFilename("digest", item.mime); filename != "digest."+item.format {
			t.Fatalf("stored filename = %s", filename)
		}
		if !validManagedResultMIME("audio", item.mime, "application/octet-stream") || validManagedResultMIME("audio", item.mime, "text/html") {
			t.Fatalf("MIME validation for %s", item.format)
		}
	}
}
