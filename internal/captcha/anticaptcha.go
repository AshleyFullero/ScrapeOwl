package captcha

import (
	"fmt"
	"time"
)

// AntiCaptcha implements the anti-captcha.com API
type AntiCaptcha struct {
	apiKey  string
	baseURL string
}

// NewAntiCaptcha creates a new anti-captcha solver
func NewAntiCaptcha(apiKey string) *AntiCaptcha {
	return &AntiCaptcha{
		apiKey:  apiKey,
		baseURL: "https://api.anti-captcha.com",
	}
}

func (ac *AntiCaptcha) Name() string { return "anticaptcha" }

// SolveImage solves an image CAPTCHA from base64
func (ac *AntiCaptcha) SolveImage(imageBase64 string) (string, error) {
	type createTaskReq struct {
		ClientKey string `json:"clientKey"`
		Task      struct {
			Type  string `json:"type"`
			Body  string `json:"body"`
		} `json:"task"`
	}
	req := createTaskReq{ClientKey: ac.apiKey}
	req.Task.Type = "ImageToTextTask"
	req.Task.Body = imageBase64

	var resp struct {
		ErrorID int    `json:"errorId"`
		TaskID  int    `json:"taskId"`
		ErrCode string `json:"errorCode"`
	}
	if err := doPost(ac.baseURL+"/createTask", req, &resp); err != nil {
		return "", fmt.Errorf("anticaptcha createTask: %w", err)
	}
	if resp.ErrorID != 0 {
		return "", fmt.Errorf("anticaptcha error: %s", resp.ErrCode)
	}
	return ac.pollResult(resp.TaskID)
}

// SolveRecaptchaV2 solves a reCAPTCHA v2 challenge
func (ac *AntiCaptcha) SolveRecaptchaV2(siteKey, pageURL string) (string, error) {
	type createTaskReq struct {
		ClientKey string `json:"clientKey"`
		Task      struct {
			Type       string `json:"type"`
			WebsiteURL string `json:"websiteURL"`
			WebsiteKey string `json:"websiteKey"`
		} `json:"task"`
	}
	req := createTaskReq{ClientKey: ac.apiKey}
	req.Task.Type = "NoCaptchaTaskProxyless"
	req.Task.WebsiteURL = pageURL
	req.Task.WebsiteKey = siteKey

	var resp struct {
		ErrorID int    `json:"errorId"`
		TaskID  int    `json:"taskId"`
		ErrCode string `json:"errorCode"`
	}
	if err := doPost(ac.baseURL+"/createTask", req, &resp); err != nil {
		return "", fmt.Errorf("anticaptcha createTask: %w", err)
	}
	if resp.ErrorID != 0 {
		return "", fmt.Errorf("anticaptcha error: %s", resp.ErrCode)
	}
	return ac.pollResult(resp.TaskID)
}

// SolveRecaptchaV3 solves a reCAPTCHA v3 challenge
func (ac *AntiCaptcha) SolveRecaptchaV3(siteKey, pageURL, action string) (string, error) {
	type createTaskReq struct {
		ClientKey string `json:"clientKey"`
		Task      struct {
			Type       string  `json:"type"`
			WebsiteURL string  `json:"websiteURL"`
			WebsiteKey string  `json:"websiteKey"`
			PageAction string  `json:"pageAction"`
			MinScore   float64 `json:"minScore"`
		} `json:"task"`
	}
	req := createTaskReq{ClientKey: ac.apiKey}
	req.Task.Type = "RecaptchaV3TaskProxyless"
	req.Task.WebsiteURL = pageURL
	req.Task.WebsiteKey = siteKey
	req.Task.PageAction = action
	req.Task.MinScore = 0.3

	var resp struct {
		ErrorID int    `json:"errorId"`
		TaskID  int    `json:"taskId"`
		ErrCode string `json:"errorCode"`
	}
	if err := doPost(ac.baseURL+"/createTask", req, &resp); err != nil {
		return "", fmt.Errorf("anticaptcha createTask: %w", err)
	}
	if resp.ErrorID != 0 {
		return "", fmt.Errorf("anticaptcha error: %s", resp.ErrCode)
	}
	return ac.pollResult(resp.TaskID)
}

// pollResult polls anti-captcha until a result is ready
func (ac *AntiCaptcha) pollResult(taskID int) (string, error) {
	type getResultReq struct {
		ClientKey string `json:"clientKey"`
		TaskID    int    `json:"taskId"`
	}

	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(3 * time.Second)

		req := getResultReq{ClientKey: ac.apiKey, TaskID: taskID}
		var resp struct {
			ErrorID  int    `json:"errorId"`
			Status   string `json:"status"`
			ErrCode  string `json:"errorCode"`
			Solution struct {
				Text    string `json:"text"`
				GToken  string `json:"gRecaptchaResponse"`
			} `json:"solution"`
		}
		if err := doPost(ac.baseURL+"/getTaskResult", req, &resp); err != nil {
			return "", fmt.Errorf("anticaptcha getTaskResult: %w", err)
		}
		if resp.ErrorID != 0 {
			return "", fmt.Errorf("anticaptcha error: %s", resp.ErrCode)
		}
		if resp.Status == "processing" {
			continue
		}
		if resp.Status == "ready" {
			if resp.Solution.GToken != "" {
				return resp.Solution.GToken, nil
			}
			return resp.Solution.Text, nil
		}
	}
	return "", fmt.Errorf("anticaptcha timeout after 120s")
}
