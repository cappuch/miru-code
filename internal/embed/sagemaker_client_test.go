package embed

import (
	"strings"
	"testing"
)

func TestParseSageMakerEndpointARN(t *testing.T) {
	region, account, name, err := ParseSageMakerEndpointARN(
		"arn:aws:sagemaker:us-west-2:123456789012:endpoint/my-embed",
	)
	if err != nil {
		t.Fatal(err)
	}
	if region != "us-west-2" || account != "123456789012" || name != "my-embed" {
		t.Fatalf("got %s %s %s", region, account, name)
	}
	if _, _, _, err := ParseSageMakerEndpointARN("not-an-arn"); err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildSageMakerBody(t *testing.T) {
	cfg := SageMakerConfig{
		EndpointName:        "ep",
		Region:              "us-east-1",
		Normalize:           true,
		Truncate:            true,
		TruncationDirection: "Right",
		PromptName:          "query",
	}
	dims := 256
	body := buildSageMakerBody(cfg, []string{"hello"}, &dims)
	if body["inputs"] == nil || body["prompt_name"] != "query" {
		t.Fatalf("%v", body)
	}
	if body["dimensions"] != 256 {
		t.Fatalf("dims=%v", body["dimensions"])
	}
}

type fakeErr string

func (e fakeErr) Error() string { return string(e) }

func TestToSageMakerInvokeError(t *testing.T) {
	cases := []struct {
		name   string
		in     error
		status int
		msg    string
	}{
		{"auth", fakeErr("AccessDeniedException: denied"), 403, SageMakerAuthErrorMessage},
		{"not found", fakeErr("ValidationError: Endpoint my-endpoint of account 123 not found."), 404, SageMakerNotFoundErrorMessage},
		{"validation other", fakeErr("ValidationError: 1 validation error detected: bad request shape StatusCode: 400"), 400, ""},
		{"unreachable dns", fakeErr("getaddrinfo ENOTFOUND runtime.sagemaker"), 503, SageMakerUnreachableErrorMessage},
		{"unreachable dial", fakeErr("dial tcp 1.2.3.4:443: connection refused"), 503, SageMakerUnreachableErrorMessage},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := toSageMakerInvokeError(tc.in)
			ie, ok := out.(*SageMakerInvokeError)
			if !ok {
				t.Fatalf("got %T", out)
			}
			if ie.Status != tc.status {
				t.Fatalf("status=%d want %d", ie.Status, tc.status)
			}
			if tc.msg != "" && ie.Message != tc.msg {
				t.Fatalf("message=%q want %q", ie.Message, tc.msg)
			}
			if tc.name == "validation other" {
				if ie.Message == SageMakerNotFoundErrorMessage {
					t.Fatal("should not map generic ValidationError to not-found")
				}
				if !strings.Contains(ie.Message, "bad request shape") {
					t.Fatalf("message=%q", ie.Message)
				}
			}
		})
	}
}
