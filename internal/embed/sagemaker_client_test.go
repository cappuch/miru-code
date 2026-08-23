package embed

import "testing"

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
