package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/klubhub/dj/api/internal/epk"
	"github.com/klubhub/dj/api/internal/platform/config"
	apphttp "github.com/klubhub/dj/api/internal/platform/http"
	"github.com/klubhub/dj/api/internal/social"
	"github.com/rs/zerolog"
)

func TestDJOriginMatrixEmailContract(t *testing.T) {
	for _, tc := range []struct {
		name, origin, site string
		forwarded, custom  bool
		want               int
	}{
		{name: "direct same origin", origin: "http://dj.example", want: 201},
		{name: "configured ingress origin", origin: "https://dj.example", want: 201},
		{name: "default port normalization", origin: "http://DJ.EXAMPLE:80", want: 201},
		{name: "nonbrowser missing Origin", want: 201},
		{name: "same-origin browser missing Origin", site: "same-origin", want: 201},
		{name: "cross-site missing Origin", site: "cross-site", want: 403},
		{name: "same-site is not same-origin", site: "same-site", want: 403},
		{name: "hostile JSON", origin: "https://hostile.example", want: 403},
		{name: "custom header is not hostile bypass", origin: "https://hostile.example", custom: true, want: 403},
		{name: "spoofed forwarding is not trusted", origin: "https://hostile.example", forwarded: true, want: 403},
		{name: "opaque", origin: "null", want: 403},
		{name: "URL path", origin: "http://dj.example/path", want: 403},
		{name: "trailing slash", origin: "http://dj.example/", want: 403},
		{name: "query", origin: "http://dj.example?", want: 403},
		{name: "fragment", origin: "http://dj.example#", want: 403},
		{name: "userinfo", origin: "http://user@dj.example", want: 403},
		{name: "list", origin: "http://dj.example https://hostile.example", want: 403},
		{name: "bad port", origin: "http://dj.example:bad", want: 403},
		{name: "wrong scheme", origin: "ftp://dj.example", want: 403},
		{name: "wrong port", origin: "http://dj.example:8080", want: 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, sender := &originEmailRepo{}, &originSender{}
			router := originRouter(repo, sender)
			r := httptest.NewRequest("POST", "http://dj.example/api/v1/finance/emails", strings.NewReader(originEmailBody))
			r.Header.Set("Content-Type", "application/json; charset=utf-8")
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			if tc.site != "" {
				r.Header.Set("Sec-Fetch-Site", tc.site)
			}
			if tc.custom {
				r.Header.Set("X-KlubHub-CSRF", "1")
			}
			if tc.forwarded {
				r.Header.Set("X-Forwarded-Host", "hostile.example")
				r.Header.Set("X-Forwarded-Proto", "https")
				r.Header.Set("Forwarded", "host=hostile.example;proto=https")
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, r)
			if rec.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", rec.Code, tc.want, rec.Body.String())
			}
			wantCalls := 0
			if tc.want == 201 {
				wantCalls = 1
			}
			if repo.creates != wantCalls || sender.calls != wantCalls {
				t.Fatalf("created=%d sent=%d want=%d", repo.creates, sender.calls, wantCalls)
			}
			if wantCalls == 1 {
				var env struct {
					Data struct {
						ID     uuid.UUID `json:"id"`
						Status string    `json:"status"`
					} `json:"data"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil || env.Data.Status != "sent" {
					t.Fatalf("invalid success envelope: %s", rec.Body.String())
				}
				get := httptest.NewRequest("GET", "http://dj.example/api/v1/finance/emails/"+env.Data.ID.String(), nil)
				get.Header.Set("Origin", "https://hostile.example")
				read := httptest.NewRecorder()
				router.ServeHTTP(read, get)
				if read.Code != 200 || !strings.Contains(read.Body.String(), env.Data.ID.String()) {
					t.Fatalf("readback: %d %s", read.Code, read.Body.String())
				}
			}
		})
	}
}

func TestDJBoundaryMethodsContract(t *testing.T) {
	for _, method := range []string{"GET", "HEAD", "OPTIONS", "POST", "PUT", "PATCH", "DELETE"} {
		t.Run(method, func(t *testing.T) {
			repo, sender := &originEmailRepo{}, &originSender{}
			r := httptest.NewRequest(method, "http://dj.example/api/v1/settings", nil)
			r.Header.Set("Origin", "null")
			rec := httptest.NewRecorder()
			originRouter(repo, sender).ServeHTTP(rec, r)
			unsafe := method == "POST" || method == "PUT" || method == "PATCH" || method == "DELETE"
			if unsafe && rec.Code != 403 {
				t.Fatalf("unsafe status=%d", rec.Code)
			}
			if !unsafe && rec.Code == 403 {
				t.Fatalf("safe status=%d", rec.Code)
			}
		})
	}
	// Missing Origin/body/type must preserve existing bodyless action callers.
	r := httptest.NewRequest("PUT", "http://dj.example/api/v1/settings", nil)
	rec := httptest.NewRecorder()
	originRouter(&originEmailRepo{}, &originSender{}).ServeHTTP(rec, r)
	if rec.Code != 204 {
		t.Fatalf("bodyless no-op status=%d", rec.Code)
	}
}

func TestDJDuplicateHeadersContract(t *testing.T) {
	for _, header := range []string{"Origin", "Content-Type"} {
		t.Run(header, func(t *testing.T) {
			repo, sender := &originEmailRepo{}, &originSender{}
			r := httptest.NewRequest("POST", "http://dj.example/api/v1/finance/emails", strings.NewReader(originEmailBody))
			r.Header.Set("Origin", "http://dj.example")
			r.Header.Set("Content-Type", "application/json")
			r.Header.Add(header, r.Header.Get(header))
			rec := httptest.NewRecorder()
			originRouter(repo, sender).ServeHTTP(rec, r)
			want := 403
			if header == "Content-Type" {
				want = 415
			}
			if rec.Code != want || repo.creates != 0 || sender.calls != 0 {
				t.Fatalf("status=%d created=%d sent=%d", rec.Code, repo.creates, sender.calls)
			}
		})
	}
}

type originEPKService struct {
	*epk.Service
	calls int
}

func (s *originEPKService) UploadPhoto(_ context.Context, data []byte, _ string) (string, error) {
	s.calls++
	return "fixture-photo", nil
}
func (s *originEPKService) PhotoURLs(_ context.Context, _ []string) map[string]string {
	return map[string]string{"fixture-photo": "/fixture-photo"}
}

type originSocialService struct {
	*social.Service
	calls int
}

func (s *originSocialService) UploadPostImage(_ context.Context, _ uuid.UUID, data []byte, _ string) (*social.PostImageResult, error) {
	s.calls++
	return &social.PostImageResult{Path: "fixture-image"}, nil
}

func TestDJMultipartRegisteredHandlerContract(t *testing.T) {
	for _, tc := range []struct {
		name, path, field string
		origin            string
		want              int
	}{
		{"epk same origin", "/api/v1/epk/photos", "photo", "http://dj.example", 201},
		{"social same origin", "/api/v1/social/posts/11111111-1111-1111-1111-111111111111/image", "image_file", "http://dj.example", 200},
		{"epk nonbrowser", "/api/v1/epk/photos", "photo", "", 201},
		{"social hostile", "/api/v1/social/posts/11111111-1111-1111-1111-111111111111/image", "image_file", "https://hostile.example", 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			es, ss := &originEPKService{}, &originSocialService{}
			noop := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
			router := apphttp.NewRouter(&config.Config{CORSOrigin: "https://dj.example"}, nil, nil, zerolog.Nop(), noop, noop, social.NewHandler(ss, nil).Routes(), epk.NewHandler(es, nil).Routes(), noop, noop, noop, nil, nil)
			var body bytes.Buffer
			mw := multipart.NewWriter(&body)
			part, err := mw.CreateFormFile(tc.field, "fixture.png")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = part.Write([]byte("local upload fixture")); err != nil {
				t.Fatal(err)
			}
			if err = mw.Close(); err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest("POST", "http://dj.example"+tc.path, &body)
			r.Header.Set("Content-Type", mw.FormDataContentType())
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, r)
			if rec.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", rec.Code, tc.want, rec.Body.String())
			}
			calls := es.calls + ss.calls
			want := 1
			if tc.want == 403 {
				want = 0
			}
			if calls != want {
				t.Fatalf("upload calls=%d want=%d", calls, want)
			}
		})
	}
}
