package extractor

import (
	"fmt"
	"regexp"

	"github.com/ashleyfullero/scrapeowl/internal/browser"
	"github.com/ashleyfullero/scrapeowl/internal/config"
)

// Result holds a single extracted value
type Result struct {
	Name  string      `json:"name"`
	Value interface{} `json:"value"`
}

// Extract runs a single extractor against the browser state
func Extract(b *browser.Browser, e config.Extractor, pageHTML string, aiCfg config.AIConfig) (*Result, error) {
	switch e.Type {
	case "css":
		return extractCSS(b, e)
	case "xpath":
		return extractXPath(b, e)
	case "ai":
		return extractAI(pageHTML, e, aiCfg)
	case "regex":
		return extractRegex(pageHTML, e)
	default:
		return nil, fmt.Errorf("unknown extractor type: %s", e.Type)
	}
}

func extractCSS(b *browser.Browser, e config.Extractor) (*Result, error) {
	attr := e.Attribute
	if attr == "" {
		attr = "text"
	}

	if e.Multiple {
		vals, err := b.GetAttributes(e.Selector, attr)
		if err != nil {
			return nil, fmt.Errorf("css extractor '%s': %w", e.Name, err)
		}
		return &Result{Name: e.Name, Value: vals}, nil
	}

	val, err := b.GetAttribute(e.Selector, attr)
	if err != nil {
		return nil, fmt.Errorf("css extractor '%s': %w", e.Name, err)
	}
	return &Result{Name: e.Name, Value: val}, nil
}

func extractXPath(b *browser.Browser, e config.Extractor) (*Result, error) {
	val, err := b.EvalXPath(e.Selector)
	if err != nil {
		return nil, fmt.Errorf("xpath extractor '%s': %w", e.Name, err)
	}
	return &Result{Name: e.Name, Value: val}, nil
}

func extractAI(pageHTML string, e config.Extractor, aiCfg config.AIConfig) (*Result, error) {
	val, err := ExtractWithAI(pageHTML, e.Prompt, aiCfg)
	if err != nil {
		return nil, fmt.Errorf("ai extractor '%s': %w", e.Name, err)
	}
	return &Result{Name: e.Name, Value: val}, nil
}

func extractRegex(pageHTML string, e config.Extractor) (*Result, error) {
	if e.Pattern == "" {
		return nil, fmt.Errorf("regex extractor '%s': pattern is required", e.Name)
	}

	re, err := regexp.Compile(e.Pattern)
	if err != nil {
		return nil, fmt.Errorf("regex extractor '%s': invalid pattern: %w", e.Name, err)
	}

	if e.Multiple {
		matches := re.FindAllString(pageHTML, -1)
		return &Result{Name: e.Name, Value: matches}, nil
	}

	match := re.FindString(pageHTML)
	return &Result{Name: e.Name, Value: match}, nil
}

// ExtractAll runs all extractors and returns a map of name → value
func ExtractAll(b *browser.Browser, extractors []config.Extractor, pageHTML string, aiCfg config.AIConfig) (map[string]interface{}, []error) {
	results := make(map[string]interface{})
	var errs []error

	for _, e := range extractors {
		r, err := Extract(b, e, pageHTML, aiCfg)
		if err != nil {
			errs = append(errs, err)
			results[e.Name] = nil
			continue
		}
		results[e.Name] = r.Value
	}

	return results, errs
}
