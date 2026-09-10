package research

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestOpenAIProviderUsesStrictStructuredOutputAndParsesResponse(t *testing.T) {
	input := AnalysisInput{PostID: "post-1", Text: "A useful post", Mechanics: CalculateMechanics("A useful post")}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-openai-key" {
			t.Errorf("authorization = %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var request map[string]any
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatal(err)
		}
		responseFormat, ok := request["response_format"].(map[string]any)
		if !ok || responseFormat["type"] != "json_schema" {
			t.Errorf("response format = %#v", request["response_format"])
		}
		jsonSchema, _ := responseFormat["json_schema"].(map[string]any)
		if jsonSchema["strict"] != true {
			t.Errorf("json schema strict = %#v", jsonSchema["strict"])
		}
		if _, ok := request["max_completion_tokens"]; !ok {
			t.Error("cost-bounded max_completion_tokens missing")
		}
		response := map[string]any{"choices": []any{map[string]any{"message": map[string]any{
			"content": mustJSON(t, map[string]any{"analyses": []any{postAnalysisResponseMap(testPostAnalysis("post-1"))}}),
		}}}, "usage": map[string]any{"prompt_tokens": 101, "completion_tokens": 17, "total_tokens": 118}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider, err := NewAnalysisProvider(AnalysisProviderConfig{
		Provider: AnalysisProviderOpenAI, Model: "test-model", APIKey: "test-openai-key", Endpoint: server.URL,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := provider.AnalyzePosts(context.Background(), []AnalysisInput{input})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Analyses) != 1 || batch.Analyses[0].PostID != "post-1" || batch.RawResponse == "" {
		t.Errorf("batch = %+v", batch)
	}
	if batch.Usage.InputTokens != 101 || batch.Usage.OutputTokens != 17 || batch.Usage.TotalTokens != 118 {
		t.Errorf("OpenAI usage = %+v", batch.Usage)
	}
}

func TestAnthropicProviderUsesMessagesStructuredOutputAndParsesResponse(t *testing.T) {
	input := AnalysisInput{PostID: "post-2", Text: "A second post", Mechanics: CalculateMechanics("A second post")}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-api-key"); got != "test-anthropic-key" {
			t.Errorf("x-api-key = %q", got)
		}
		if got := r.Header.Get("anthropic-version"); got != "2023-06-01" {
			t.Errorf("anthropic-version = %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var request map[string]any
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatal(err)
		}
		outputConfig, _ := request["output_config"].(map[string]any)
		if outputConfig == nil {
			t.Error("Anthropic output_config missing")
		}
		response := map[string]any{"content": []any{map[string]any{
			"type": "text", "text": mustJSON(t, map[string]any{"analyses": []any{postAnalysisResponseMap(testPostAnalysis("post-2"))}}),
		}}, "usage": map[string]any{"input_tokens": 103, "output_tokens": 19}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider, err := NewAnalysisProvider(AnalysisProviderConfig{
		Provider: AnalysisProviderAnthropic, Model: "test-model", APIKey: "test-anthropic-key", Endpoint: server.URL,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := provider.AnalyzePosts(context.Background(), []AnalysisInput{input})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Analyses) != 1 || batch.Analyses[0].PostID != "post-2" {
		t.Errorf("batch = %+v", batch)
	}
	if batch.Usage.InputTokens != 103 || batch.Usage.OutputTokens != 19 || batch.Usage.TotalTokens != 122 {
		t.Errorf("Anthropic usage = %+v", batch.Usage)
	}
}

func TestHTTPAnalysisProviderSurfacesHTTPFailuresAndCancellation(t *testing.T) {
	for _, status := range []int{401, 429, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				_, _ = io.WriteString(w, `{"error":"fixture"}`)
			}))
			defer server.Close()
			provider, err := NewAnalysisProvider(AnalysisProviderConfig{
				Provider: AnalysisProviderOpenAI, Model: "test-model", APIKey: "test-key", Endpoint: server.URL,
			}, nil)
			if err != nil {
				t.Fatal(err)
			}
			_, err = provider.AnalyzePosts(context.Background(), []AnalysisInput{{PostID: "post-1", Text: "text"}})
			if err == nil || !strings.Contains(err.Error(), "HTTP "+strconv.Itoa(status)) {
				t.Errorf("status %d error = %v", status, err)
			}
		})
	}

	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{}`)
	}))
	defer providerServer.Close()
	provider, err := NewAnalysisProvider(AnalysisProviderConfig{
		Provider: AnalysisProviderOpenAI, Model: "test-model", APIKey: "test-key", Endpoint: providerServer.URL,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := provider.AnalyzePosts(ctx, []AnalysisInput{{PostID: "post-1", Text: "text"}}); !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("cancellation error = %v", err)
	}
}

func TestAnalysisResponseValidationRejectsMalformedAndAllowsPartialBatches(t *testing.T) {
	inputs := []AnalysisInput{{PostID: "post-1"}, {PostID: "post-2"}}
	valid := testPostAnalysis("post-1")
	malformedBody := map[string]any{"analyses": []any{map[string]any{
		"post_id": valid.PostID, "content_type": valid.ContentType, "content_type_confidence": valid.ContentTypeConfidence,
		"hook_type": valid.HookType, "hook_confidence": valid.HookConfidence, "structure": valid.Structure,
		"structure_confidence": valid.StructureConfidence, "tone": valid.Tone, "tone_confidence": valid.ToneConfidence,
		"intent": valid.Intent, "intent_confidence": valid.IntentConfidence, "main_topic": valid.MainTopic,
		"main_topic_confidence": valid.MainTopicConfidence, "subtopics": valid.Subtopics, "pains": valid.Pains,
		"desires": valid.Desires, "questions": valid.Questions, "tools_products_services": valid.ToolsProductsServices,
		"target_audience": valid.TargetAudience, "target_audience_confidence": valid.TargetAudienceConfidence,
		"claims": valid.Claims, "cta": valid.CTA, "extra": "reject me",
	}}}
	malformed := strings.ReplaceAll(mustJSON(t, malformedBody), "\n", "")
	if _, err := parsePostAnalysisResponse(malformed, inputs); err == nil {
		t.Error("malformed response was accepted")
	}

	partial := mustJSON(t, map[string]any{"analyses": []any{postAnalysisResponseMap(valid)}})
	analyses, err := parsePostAnalysisResponse(partial, inputs)
	if err != nil || len(analyses) != 1 {
		t.Fatalf("partial response: analyses=%+v err=%v", analyses, err)
	}
	missingField := postAnalysisResponseMap(valid)
	delete(missingField, "intent")
	if _, err := parsePostAnalysisResponse(mustJSON(t, map[string]any{"analyses": []any{missingField}}), inputs); err == nil {
		t.Error("response with a missing required field was accepted")
	}

	invalidEnum := valid
	invalidEnum.ContentType = "invented_type"
	if _, err := parsePostAnalysisResponse(mustJSON(t, map[string]any{"analyses": []any{postAnalysisResponseMap(invalidEnum)}}), inputs); err == nil {
		t.Error("unsupported enum was accepted")
	}
}

func TestAnalysisProviderConfigFromEnvIsExplicitAndCredentialSafe(t *testing.T) {
	t.Setenv("THREADS_ANALYSIS_PROVIDER", "anthropic")
	t.Setenv("THREADS_ANALYSIS_MODEL", "env-model")
	t.Setenv("ANTHROPIC_API_KEY", "env-secret")
	t.Setenv("OPENAI_API_KEY", "")
	config, err := AnalysisProviderConfigFromEnv("", "")
	if err != nil {
		t.Fatal(err)
	}
	if config.Provider != AnalysisProviderAnthropic || config.Model != "env-model" || config.APIKey != "env-secret" {
		t.Errorf("config = %+v", config)
	}

	t.Setenv("ANTHROPIC_API_KEY", "")
	if _, err := AnalysisProviderConfigFromEnv("anthropic", "model"); err == nil || !strings.Contains(err.Error(), "API key") {
		t.Errorf("missing-key error = %v", err)
	}
}

func TestValidatePostAnalysisNormalizesUnknownAndSecondaryTypes(t *testing.T) {
	analysis := testPostAnalysis("post-unknown")
	analysis.ContentType = "UNKNOWN"
	analysis.SecondaryContentTypes = []string{"Educational", "unknown", "educational"}
	analysis.MainTopic = ""
	if err := ValidatePostAnalysis(&analysis); err != nil {
		t.Fatal(err)
	}
	if analysis.ContentType != UnknownLabel || analysis.MainTopic != UnknownLabel {
		t.Errorf("unknown values = content=%q topic=%q", analysis.ContentType, analysis.MainTopic)
	}
	if len(analysis.SecondaryContentTypes) != 1 || analysis.SecondaryContentTypes[0] != "educational" {
		t.Errorf("secondary types = %v", analysis.SecondaryContentTypes)
	}
	analysis.SecondaryContentTypes = []string{"unsupported"}
	if err := ValidatePostAnalysis(&analysis); err == nil {
		t.Error("unsupported secondary content type was accepted")
	}
}

func testPostAnalysis(postID string) PostAnalysis {
	return PostAnalysis{
		PostID: postID, ContentType: "educational", ContentTypeConfidence: 0.9,
		HookType: "strong_claim", HookConfidence: 0.8, Structure: "hook_explanation", StructureConfidence: 0.8,
		Tone: []string{"educational", "conversational"}, ToneConfidence: 0.8,
		Intent: "share_knowledge", IntentConfidence: 0.9, MainTopic: "automation", MainTopicConfidence: 0.8,
		Subtopics: []string{"workflow"}, Pains: []string{"finding clients is hard"},
		Desires: []string{"more clients"}, Questions: []string{"How do I find clients?"},
		ToolsProductsServices: []string{"n8n"}, TargetAudience: "freelancers", TargetAudienceConfidence: 0.7,
		Claims: []string{"automation saves time"}, CTA: "reply with your workflow",
	}
}

func postAnalysisResponseMap(value PostAnalysis) map[string]any {
	secondary := value.SecondaryContentTypes
	if secondary == nil {
		secondary = []string{}
	}
	return map[string]any{
		"post_id": value.PostID, "content_type": value.ContentType, "content_type_confidence": value.ContentTypeConfidence,
		"secondary_content_types": secondary,
		"hook_type":               value.HookType, "hook_confidence": value.HookConfidence, "structure": value.Structure,
		"structure_confidence": value.StructureConfidence, "tone": value.Tone, "tone_confidence": value.ToneConfidence,
		"intent": value.Intent, "intent_confidence": value.IntentConfidence, "main_topic": value.MainTopic,
		"main_topic_confidence": value.MainTopicConfidence, "subtopics": value.Subtopics, "pains": value.Pains,
		"desires": value.Desires, "questions": value.Questions, "tools_products_services": value.ToolsProductsServices,
		"target_audience": value.TargetAudience, "target_audience_confidence": value.TargetAudienceConfidence,
		"claims": value.Claims, "cta": value.CTA,
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
