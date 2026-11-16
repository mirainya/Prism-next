package open

import (
	"testing"
	"time"

	"github.com/mirainya/Prism/internal/gateway/repository"
)

func TestUnifiedImageWaitHonorsLongerCatalogDeadline(t *testing.T) {
	for _, example := range []struct {
		name      string
		timeoutMS uint64
		want      time.Duration
	}{
		{"default", 0, 5 * time.Minute},
		{"short_transport", 30000, 5 * time.Minute},
		{"existing_transport", 300000, 5 * time.Minute},
		{"long_image_generation", 600000, 10 * time.Minute},
		{"maximum", ^uint64(0), 15 * time.Minute},
	} {
		t.Run(example.name, func(t *testing.T) {
			got := unifiedOpenAIImageWaitTimeout(repository.RoutePolicy{TransportTimeoutMS: example.timeoutMS})
			if got != example.want {
				t.Fatalf("image wait = %s, want %s", got, example.want)
			}
		})
	}
}
