package open

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/internal/gateway/routing"
	gatewayruntime "github.com/mirainya/Prism/internal/gateway/runtime"
)

func TestMediaCreationErrorsIdentifyOnlyKnownRejections(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []string{"image", "video"} {
		for _, tc := range []struct {
			name, callID, location string
			err                    error
			rejected               bool
		}{
			{name: "billing rejected before call creation", err: fmt.Errorf("wrapped: %w: %w", gatewayruntime.ErrSubmissionRejected, repository.ErrNotFound), rejected: true},
			{name: "no compatible route", err: routing.ErrNoRoute, rejected: true},
			{name: "not ready", err: gatewayruntime.ErrNotReady, rejected: true},
			{name: "ambiguous commit", err: errors.New("commit gateway transaction: connection lost")},
			{name: "deadline exceeded", err: context.DeadlineExceeded},
			{name: "unclassified missing row", err: repository.ErrNotFound},
			{name: "accepted call wins", err: gatewayruntime.ErrSubmissionRejected, callID: "accepted-call"},
			{name: "accepted location wins", err: gatewayruntime.ErrSubmissionRejected, location: "/v1/images/tasks/accepted"},
		} {
			t.Run(endpoint+"/"+tc.name, func(t *testing.T) {
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Header(prismCallIDHeader, tc.callID)
				c.Header("Location", tc.location)
				if endpoint == "image" {
					writeOpenAIImageExecutionError(c, tc.err)
				} else {
					writeVideoCreateError(c, http.StatusInternalServerError, tc.err)
				}
				if got := recorder.Header().Get("X-Prism-Submission-Status") == "rejected"; got != tc.rejected {
					t.Fatalf("rejected = %v, want %v", got, tc.rejected)
				}
				if recorder.Code < 400 {
					t.Fatalf("unexpected success: %d", recorder.Code)
				}
			})
		}
	}
}
