package research

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	defaultOpenAIModel       = "gpt-4.1-mini"
	defaultAnthropicModel    = "claude-haiku-4-5"
	defaultProviderTimeout   = 2 * time.Minute
	maxProviderResponseBytes = 8 << 20
	maxAnalysisOutputTokens  = 6000
)

// AnalysisProviderConfig is intentionally plain so a caller can configure an
// endpoint in tests or point a compatible gateway at the same adapter.
type AnalysisProviderConfig struct {
	Provider string
	Model    string
	APIKey   string
	Endpoint string
	Timeout  time.Duration
}

// AnalysisProviderConfigFromEnv resolves provider settings without ever
// printing the credential. THREADS_ANALYSIS_* overrides provider-specific
// variables, which makes local and CI configuration explicit.
func AnalysisProviderConfigFromEnv(providerOverride, modelOverride string) (AnalysisProviderConfig, error) {
	provider := strings.ToLower(strings.TrimSpace(providerOverride))
	if provider == "" {
		provider = strings.ToLower(strings.TrimSpace(firstNonEmptyEnv(
			"THREADS_ANALYSIS_PROVIDER", "THREADS_RESEARCH_ANALYSIS_PROVIDER",
		)))
	}
	if provider == "" {
		switch {
		case strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) != "":
			provider = AnalysisProviderOpenAI
		case strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")) != "":
			provider = AnalysisProviderAnthropic
		}
	}
	if provider != AnalysisProviderOpenAI && provider != AnalysisProviderAnthropic {
		return AnalysisProviderConfig{}, fmt.Errorf("analysis provider must be %q or %q", AnalysisProviderOpenAI, AnalysisProviderAnthropic)
	}

	model := strings.TrimSpace(modelOverride)
	if model == "" {
		model = strings.TrimSpace(firstNonEmptyEnv("THREADS_ANALYSIS_MODEL", "THREADS_RESEARCH_ANALYSIS_MODEL"))
	}
	if model == "" && provider == AnalysisProviderOpenAI {
		model = strings.TrimSpace(os.Getenv("OPENAI_MODEL"))
	}
	if model == "" && provider == AnalysisProviderAnthropic {
		model = strings.TrimSpace(os.Getenv("ANTHROPIC_MODEL"))
	}
	if model == "" {
		if provider == AnalysisProviderOpenAI {
			model = defaultOpenAIModel
		} else {
			model = defaultAnthropicModel
		}
	}

	key := strings.TrimSpace(os.Getenv("THREADS_ANALYSIS_API_KEY"))
	if key == "" {
		if provider == AnalysisProviderOpenAI {
			key = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
		} else {
			key = strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
		}
	}
	if key == "" {
		return AnalysisProviderConfig{}, fmt.Errorf("analysis API key is missing for provider %q", provider)
	}

	endpoint := strings.TrimSpace(firstNonEmptyEnv("THREADS_ANALYSIS_ENDPOINT", "THREADS_ANALYSIS_BASE_URL"))
	if endpoint == "" {
		if provider == AnalysisProviderOpenAI {
			endpoint = "https://api.openai.com/v1/chat/completions"
		} else {
			endpoint = "https://api.anthropic.com/v1/messages"
		}
	}
	return AnalysisProviderConfig{
		Provider: provider,
		Model:    model,
		APIKey:   key,
		Endpoint: endpoint,
		Timeout:  defaultProviderTimeout,
	}, nil
}

func firstNonEmptyEnv(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

// NewAnalysisProvider constructs one of the two supported direct HTTP
// adapters. No SDK is required, keeping the Go CLI's dependency surface small.
func NewAnalysisProvider(config AnalysisProviderConfig, client *http.Client) (AnalysisProvider, error) {
	config.Provider = strings.ToLower(strings.TrimSpace(config.Provider))
	config.Model = strings.TrimSpace(config.Model)
	config.APIKey = strings.TrimSpace(config.APIKey)
	config.Endpoint = strings.TrimSpace(config.Endpoint)
	if config.Provider != AnalysisProviderOpenAI && config.Provider != AnalysisProviderAnthropic {
		return nil, fmt.Errorf("unsupported analysis provider %q", config.Provider)
	}
	if config.Model == "" || config.APIKey == "" || config.Endpoint == "" {
		return nil, errors.New("analysis provider model, API key, and endpoint are required")
	}
	if client == nil {
		timeout := config.Timeout
		if timeout <= 0 {
			timeout = defaultProviderTimeout
		}
		client = &http.Client{Timeout: timeout}
	}
	return &httpAnalysisProvider{config: config, client: client}, nil
}

type httpAnalysisProvider struct {
	config AnalysisProviderConfig
	client *http.Client
}

func (p *httpAnalysisProvider) Name() string  { return p.config.Provider }
func (p *httpAnalysisProvider) Model() string { return p.config.Model }

func (p *httpAnalysisProvider) AnalyzePosts(ctx context.Context, inputs []AnalysisInput) (AnalysisBatch, error) {
	if len(inputs) == 0 {
		return AnalysisBatch{}, errors.New("at least one analysis input is required")
	}
	for i := range inputs {
		inputs[i].PostID = strings.TrimSpace(inputs[i].PostID)
		if inputs[i].PostID == "" {
			return AnalysisBatch{}, fmt.Errorf("analysis input %d has no post_id", i)
		}
	}
	requestText, err := marshalPostPrompt(inputs)
	if err != nil {
		return AnalysisBatch{}, err
	}
	content, raw, err := p.requestJSON(ctx, analysisSystemPrompt, requestText, postAnalysisSchema())
	if err != nil {
		return AnalysisBatch{}, err
	}
	analyses, err := parsePostAnalysisResponse(content, inputs)
	if err != nil {
		return AnalysisBatch{}, err
	}
	return AnalysisBatch{Analyses: analyses, RawResponse: raw, Usage: analysisUsageFromResponse(p.config.Provider, raw)}, nil
}

func analysisUsageFromResponse(provider, raw string) AnalysisUsage {
	var envelope struct {
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
			InputTokens      int `json:"input_tokens"`
			OutputTokens     int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return AnalysisUsage{}
	}
	usage := AnalysisUsage{}
	if provider == AnalysisProviderAnthropic {
		usage.InputTokens = envelope.Usage.InputTokens
		usage.OutputTokens = envelope.Usage.OutputTokens
	} else {
		usage.InputTokens = envelope.Usage.PromptTokens
		usage.OutputTokens = envelope.Usage.CompletionTokens
	}
	usage.TotalTokens = envelope.Usage.TotalTokens
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}
	return usage
}

func (p *httpAnalysisProvider) AnalyzeAggregatePatterns(ctx context.Context, input AggregatePatternInput) (AggregatePatternResult, error) {
	if strings.TrimSpace(input.Topic) == "" {
		return AggregatePatternResult{}, errors.New("aggregate analysis topic is required")
	}
	requestText, err := json.Marshal(input)
	if err != nil {
		return AggregatePatternResult{}, fmt.Errorf("marshal aggregate analysis input: %w", err)
	}
	content, raw, err := p.requestJSON(ctx, aggregateSystemPrompt, string(requestText), aggregatePatternSchema())
	if err != nil {
		return AggregatePatternResult{}, err
	}
	var result AggregatePatternResult
	if err := decodeStrictJSON([]byte(content), &result); err != nil {
		return AggregatePatternResult{}, fmt.Errorf("decode aggregate analysis response: %w", err)
	}
	return AggregatePatternResult{Summary: strings.TrimSpace(result.Summary), Observations: result.Observations, RawResponse: raw}, nil
}

func (p *httpAnalysisProvider) requestJSON(ctx context.Context, systemPrompt, userPrompt string, schema map[string]any) (content, raw string, err error) {
	var body any
	switch p.config.Provider {
	case AnalysisProviderOpenAI:
		body = map[string]any{
			"model": p.config.Model,
			"messages": []map[string]string{
				{"role": "system", "content": systemPrompt},
				{"role": "user", "content": userPrompt},
			},
			"temperature":           0,
			"max_completion_tokens": maxAnalysisOutputTokens,
			"response_format": map[string]any{
				"type": "json_schema",
				"json_schema": map[string]any{
					"name":   "threads_content_intelligence",
					"strict": true,
					"schema": schema,
				},
			},
		}
	case AnalysisProviderAnthropic:
		body = map[string]any{
			"model":      p.config.Model,
			"max_tokens": maxAnalysisOutputTokens,
			"system":     systemPrompt,
			"messages": []map[string]string{
				{"role": "user", "content": userPrompt},
			},
			"temperature": 0,
			"output_config": map[string]any{
				"format": map[string]any{
					"type":   "json_schema",
					"schema": schema,
				},
			},
		}
	default:
		return "", "", fmt.Errorf("unsupported analysis provider %q", p.config.Provider)
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", "", fmt.Errorf("marshal %s analysis request: %w", p.config.Provider, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.config.Endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", "", fmt.Errorf("create %s analysis request: %w", p.config.Provider, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.config.Provider == AnalysisProviderOpenAI {
		req.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	} else {
		req.Header.Set("x-api-key", p.config.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("%s analysis request: %w", p.config.Provider, err)
	}
	defer func() { _ = resp.Body.Close() }()
	rawBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxProviderResponseBytes))
	if err != nil {
		return "", "", fmt.Errorf("read %s analysis response: %w", p.config.Provider, err)
	}
	raw = string(rawBytes)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", raw, fmt.Errorf("%s analysis request returned HTTP %d: %s", p.config.Provider, resp.StatusCode, compactErrorBody(raw))
	}
	if p.config.Provider == AnalysisProviderOpenAI {
		content, err = openAIContent(rawBytes)
	} else {
		content, err = anthropicContent(rawBytes)
	}
	if err != nil {
		return "", raw, fmt.Errorf("decode %s analysis response: %w", p.config.Provider, err)
	}
	return content, raw, nil
}

func marshalPostPrompt(inputs []AnalysisInput) (string, error) {
	// The input shape is deliberately explicit in the prompt. Performance
	// fields never enter this JSON, and mechanics are supplied as observations
	// rather than requested model judgments.
	payload := struct {
		Posts []AnalysisInput `json:"posts"`
	}{Posts: inputs}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal post analysis input: %w", err)
	}
	return "Classify every post in this JSON. Return one item per post_id and no other text.\n" + string(b), nil
}

func parsePostAnalysisResponse(content string, inputs []AnalysisInput) ([]PostAnalysis, error) {
	var envelope struct {
		Analyses []PostAnalysis `json:"analyses"`
	}
	if err := decodeStrictJSON([]byte(content), &envelope); err != nil {
		return nil, fmt.Errorf("decode post analysis response: %w", err)
	}
	if len(envelope.Analyses) == 0 {
		return nil, errors.New("analysis response contains no analyses")
	}
	if err := validateRequiredAnalysisFields(content, len(envelope.Analyses)); err != nil {
		return nil, err
	}
	if err := validateAnalysisIDs(envelope.Analyses, inputs); err != nil {
		return nil, err
	}
	return envelope.Analyses, nil
}

func validateRequiredAnalysisFields(content string, count int) error {
	var envelope struct {
		Analyses []map[string]json.RawMessage `json:"analyses"`
	}
	if err := json.Unmarshal([]byte(content), &envelope); err != nil {
		return fmt.Errorf("decode analysis field presence: %w", err)
	}
	if len(envelope.Analyses) != count {
		return errors.New("analysis response contains an invalid analyses array")
	}
	for index, item := range envelope.Analyses {
		for _, field := range postAnalysisRequired() {
			if _, ok := item[field]; !ok {
				return fmt.Errorf("analysis item %d is missing required field %q", index, field)
			}
		}
	}
	return nil
}

func decodeStrictJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values in response")
		}
		return err
	}
	return nil
}

func openAIContent(data []byte) (string, error) {
	var envelope struct {
		Choices []struct {
			Message struct {
				Content json.RawMessage `json:"content"`
				Refusal string          `json:"refusal"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return "", err
	}
	if len(envelope.Choices) == 0 {
		return "", errors.New("OpenAI response contains no choices")
	}
	if envelope.Choices[0].Message.Refusal != "" {
		return "", fmt.Errorf("model refusal: %s", envelope.Choices[0].Message.Refusal)
	}
	var content string
	if err := json.Unmarshal(envelope.Choices[0].Message.Content, &content); err == nil {
		if strings.TrimSpace(content) == "" {
			return "", errors.New("OpenAI response content is empty")
		}
		return content, nil
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(envelope.Choices[0].Message.Content, &parts); err != nil {
		return "", fmt.Errorf("unsupported OpenAI message content: %w", err)
	}
	var out strings.Builder
	for _, part := range parts {
		if part.Type == "text" {
			out.WriteString(part.Text)
		}
	}
	if strings.TrimSpace(out.String()) == "" {
		return "", errors.New("OpenAI response content is empty")
	}
	return out.String(), nil
}

func anthropicContent(data []byte) (string, error) {
	var envelope struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return "", err
	}
	var out strings.Builder
	for _, block := range envelope.Content {
		if block.Type == "text" {
			out.WriteString(block.Text)
		}
	}
	if strings.TrimSpace(out.String()) == "" {
		return "", errors.New("anthropic response contains no text content")
	}
	return out.String(), nil
}

func compactErrorBody(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len([]rune(value)) > 500 {
		return string([]rune(value)[:500]) + "..."
	}
	return value
}

const analysisSystemPrompt = `You classify public Threads posts for a research report. Use only the post text and its observable writing mechanics. Do not infer performance, virality, market size, demand, or business outcomes. Use the exact enum labels in the JSON schema. Choose one primary content_type and use secondary_content_types only for clearly supported additional types; it may be an empty array. Use "unknown" when the text does not support a label. Confidence is your confidence in the semantic label, from 0 to 1. Extract pains, desires, questions, tools/products/services, claims, and CTA only when explicitly supported by the post; otherwise use an empty array or "unknown". Return exactly the requested JSON object.`

const aggregateSystemPrompt = `You summarize already-classified Threads evidence. Do not invent facts, performance, demand, or business claims. Return concise observations grounded in the supplied classified posts. This result is optional narrative context; deterministic code remains the source of truth for counts, medians, quartiles, thresholds, and evidence links.`

func postAnalysisSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"analyses": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"properties":           postAnalysisProperties(),
					"required":             postAnalysisRequired(),
				},
			},
		},
		"required": []string{"analyses"},
	}
}

func postAnalysisProperties() map[string]any {
	return map[string]any{
		"post_id":                    stringSchema(),
		"content_type":               enumSchema(contentTypeLabels),
		"content_type_confidence":    confidenceSchema(),
		"secondary_content_types":    enumListSchema(contentTypeLabels, 4),
		"hook_type":                  enumSchema(hookLabels),
		"hook_confidence":            confidenceSchema(),
		"structure":                  enumSchema(structureLabels),
		"structure_confidence":       confidenceSchema(),
		"tone":                       enumArraySchema(toneLabels),
		"tone_confidence":            confidenceSchema(),
		"intent":                     enumSchema(intentLabels),
		"intent_confidence":          confidenceSchema(),
		"main_topic":                 stringSchema(),
		"main_topic_confidence":      confidenceSchema(),
		"subtopics":                  stringArraySchema(),
		"pains":                      stringArraySchema(),
		"desires":                    stringArraySchema(),
		"questions":                  stringArraySchema(),
		"tools_products_services":    stringArraySchema(),
		"target_audience":            stringSchema(),
		"target_audience_confidence": confidenceSchema(),
		"claims":                     stringArraySchema(),
		"cta":                        stringSchema(),
	}
}

func postAnalysisRequired() []string {
	return []string{
		"post_id", "content_type", "content_type_confidence", "secondary_content_types", "hook_type", "hook_confidence",
		"structure", "structure_confidence", "tone", "tone_confidence", "intent", "intent_confidence",
		"main_topic", "main_topic_confidence", "subtopics", "pains", "desires", "questions",
		"tools_products_services", "target_audience", "target_audience_confidence", "claims", "cta",
	}
}

func aggregatePatternSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"summary":      stringSchema(),
			"observations": stringArraySchema(),
		},
		"required": []string{"summary", "observations"},
	}
}

func stringSchema() map[string]any {
	return map[string]any{"type": "string", "maxLength": 300}
}

func stringArraySchema() map[string]any {
	return map[string]any{"type": "array", "maxItems": 20, "items": stringSchema()}
}

func confidenceSchema() map[string]any {
	return map[string]any{"type": "number", "minimum": 0, "maximum": 1}
}

func enumSchema(labels map[string]bool) map[string]any {
	values := sortedLabels(labels)
	return map[string]any{"type": "string", "enum": values}
}

func enumArraySchema(labels map[string]bool) map[string]any {
	values := sortedLabels(labels)
	return map[string]any{"type": "array", "minItems": 1, "maxItems": 4, "items": map[string]any{"type": "string", "enum": values}}
}

func enumListSchema(labels map[string]bool, maxItems int) map[string]any {
	values := sortedLabels(labels)
	return map[string]any{"type": "array", "minItems": 0, "maxItems": maxItems, "items": map[string]any{"type": "string", "enum": values}}
}

func sortedLabels(labels map[string]bool) []string {
	values := make([]string, 0, len(labels))
	for value := range labels {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}
