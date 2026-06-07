package captcha

import (
	"fmt"
	"net/url"
	"time"
)

// TwoCaptcha implements the 2captcha.com API
type TwoCaptcha struct {
	apiKey  string
	baseURL string
}

// NewTwoCaptcha creates a new 2captcha solver
func NewTwoCaptcha(apiKey string) *TwoCaptcha {
	return &TwoCaptcha{
		apiKey:  apiKey,
		baseURL: "https://2captcha.com",
	}
}

func (tc *TwoCaptcha) Name() string { return "2captcha" }

// SolveImage solves an image CAPTCHA from base64
func (tc *TwoCaptcha) SolveImage(imageBase64 string) (string, error) {
	// Submit task
	submitURL := fmt.Sprintf("%s/in.php?key=%s&method=base64&body=%s&json=1",
		tc.baseURL, tc.apiKey, url.QueryEscape(imageBase64))

	var submitResp struct {
		Status  int    `json:"status"`
		Request string `json:"request"`
	}
	if err := doPost(submitURL, nil, &submitResp); err != nil {
		return "", fmt.Errorf("2captcha submit: %w", err)
	}
	if submitResp.Status != 1 {
		return "", fmt.Errorf("2captcha submit error: %s", submitResp.Request)
	}

	taskID := submitResp.Request
	return tc.pollResult(taskID)
}

// SolveRecaptchaV2 solves a reCAPTCHA v2 challenge
func (tc *TwoCaptcha) SolveRecaptchaV2(siteKey, pageURL string) (string, error) {
	submitURL := fmt.Sprintf("%s/in.php?key=%s&method=userrecaptcha&googlekey=%s&pageurl=%s&json=1",
		tc.baseURL, tc.apiKey, siteKey, url.QueryEscape(pageURL))

	var submitResp struct {
		Status  int    `json:"status"`
		Request string `json:"request"`
	}
	if err := doPost(submitURL, nil, &submitResp); err != nil {
		return "", fmt.Errorf("2captcha submit: %w", err)
	}
	if submitResp.Status != 1 {
		return "", fmt.Errorf("2captcha submit error: %s", submitResp.Request)
	}

	return tc.pollResult(submitResp.Request)
}

// SolveRecaptchaV3 solves a reCAPTCHA v3 challenge
func (tc *TwoCaptcha) SolveRecaptchaV3(siteKey, pageURL, action string) (string, error) {
	submitURL := fmt.Sprintf("%s/in.php?key=%s&method=userrecaptcha&version=v3&googlekey=%s&pageurl=%s&action=%s&json=1",
		tc.baseURL, tc.apiKey, siteKey, url.QueryEscape(pageURL), action)

	var submitResp struct {
		Status  int    `json:"status"`
		Request string `json:"request"`
	}
	if err := doPost(submitURL, nil, &submitResp); err != nil {
		return "", fmt.Errorf("2captcha v3 submit: %w", err)
	}
	if submitResp.Status != 1 {
		return "", fmt.Errorf("2captcha v3 error: %s", submitResp.Request)
	}

	return tc.pollResult(submitResp.Request)
}

// pollResult polls 2captcha until a result is ready (up to 120s)
func (tc *TwoCaptcha) pollResult(taskID string) (string, error) {
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(5 * time.Second)

		resultURL := fmt.Sprintf("%s/res.php?key=%s&action=get&id=%s&json=1",
			tc.baseURL, tc.apiKey, taskID)

		var result struct {
			Status  int    `json:"status"`
			Request string `json:"request"`
		}
		if err := doPost(resultURL, nil, &result); err != nil {
			return "", fmt.Errorf("2captcha poll: %w", err)
		}
		if result.Request == "CAPCHA_NOT_READY" {
			continue
		}
		if result.Status == 1 {
			return result.Request, nil
		}
		return "", fmt.Errorf("2captcha error: %s", result.Request)
	}
	return "", fmt.Errorf("2captcha timeout after 120s")
}
