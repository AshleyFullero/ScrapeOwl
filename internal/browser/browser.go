package browser

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/ashleyfullero/scrapeowl/internal/config"
)

// Browser manages a Chrome browser instance via CDP
type Browser struct {
	allocCtx    context.Context
	allocCancel context.CancelFunc
	ctx         context.Context
	cancel      context.CancelFunc
	logger      *slog.Logger
}

// Options configures the browser instance
type Options struct {
	Headless    bool
	ProxyURL    string
	UserAgent   string
	WindowWidth int
	WindowHeight int
	DisableImages bool
}

// DefaultOptions returns sensible browser defaults
func DefaultOptions() Options {
	return Options{
		Headless:     true,
		WindowWidth:  1920,
		WindowHeight: 1080,
	}
}

// New creates a new browser instance
func New(opts Options, logger *slog.Logger) (*Browser, error) {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}

	allocOpts := chromedp.DefaultExecAllocatorOptions[:]

	if opts.Headless {
		allocOpts = append(allocOpts,
			chromedp.Flag("headless", true),
			chromedp.Flag("disable-gpu", true),
		)
	}

	// Security and stability flags
	allocOpts = append(allocOpts,
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-setuid-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-web-security", false),
		chromedp.Flag("ignore-certificate-errors", true),
		chromedp.WindowSize(opts.WindowWidth, opts.WindowHeight),
	)

	if opts.UserAgent != "" {
		allocOpts = append(allocOpts, chromedp.UserAgent(opts.UserAgent))
	}

	if opts.ProxyURL != "" {
		allocOpts = append(allocOpts,
			chromedp.ProxyServer(opts.ProxyURL),
		)
	}

	if opts.DisableImages {
		allocOpts = append(allocOpts,
			chromedp.Flag("blink-settings", "imagesEnabled=false"),
		)
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), allocOpts...)
	ctx, cancel := chromedp.NewContext(allocCtx,
		chromedp.WithLogf(func(format string, args ...interface{}) {
			logger.Debug(fmt.Sprintf(format, args...))
		}),
	)

	// Start browser by navigating to blank page (ensures Chrome is launched)
	if err := chromedp.Run(ctx, chromedp.Navigate("about:blank")); err != nil {
		cancel()
		allocCancel()
		return nil, fmt.Errorf("starting browser: %w", err)
	}

	return &Browser{
		allocCtx:    allocCtx,
		allocCancel: allocCancel,
		ctx:         ctx,
		cancel:      cancel,
		logger:      logger,
	}, nil
}

// Close shuts down the browser and all associated resources
func (b *Browser) Close() {
	if b.cancel != nil {
		b.cancel()
	}
	if b.allocCancel != nil {
		b.allocCancel()
	}
}

// RunStep executes a single job step in the browser
func (b *Browser) RunStep(step config.Step) error {
	timeout := 30 * time.Second
	if step.Timeout > 0 {
		timeout = step.Timeout
	}

	ctx, cancel := context.WithTimeout(b.ctx, timeout)
	defer cancel()

	var tasks []chromedp.Action

	switch step.Action {
	case "navigate":
		target := step.URL
		if target == "" {
			target = step.Selector // allow selector field to hold URL for navigate
		}
		tasks = append(tasks, chromedp.Navigate(target))

	case "click":
		if step.Selector == "" {
			return fmt.Errorf("click action requires a selector")
		}
		tasks = append(tasks,
			chromedp.WaitVisible(step.Selector, chromedp.ByQuery),
			chromedp.Click(step.Selector, chromedp.ByQuery),
		)

	case "type":
		if step.Selector == "" {
			return fmt.Errorf("type action requires a selector")
		}
		tasks = append(tasks,
			chromedp.WaitVisible(step.Selector, chromedp.ByQuery),
			chromedp.Click(step.Selector, chromedp.ByQuery),
			chromedp.SendKeys(step.Selector, step.Text, chromedp.ByQuery),
		)

	case "clear":
		if step.Selector == "" {
			return fmt.Errorf("clear action requires a selector")
		}
		tasks = append(tasks,
			chromedp.WaitVisible(step.Selector, chromedp.ByQuery),
			chromedp.Clear(step.Selector, chromedp.ByQuery),
		)

	case "hover":
		if step.Selector == "" {
			return fmt.Errorf("hover action requires a selector")
		}
		tasks = append(tasks,
			chromedp.WaitVisible(step.Selector, chromedp.ByQuery),
			chromedp.MouseOver(step.Selector, chromedp.ByQuery),
		)

	case "scroll":
		if step.Selector != "" {
			tasks = append(tasks,
				chromedp.ScrollIntoView(step.Selector, chromedp.ByQuery),
			)
		} else {
			tasks = append(tasks,
				chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight)`, nil),
			)
		}

	case "screenshot":
		var buf []byte
		path := step.Selector // use selector as path
		if path == "" {
			path = "./output/screenshot.png"
		}
		tasks = append(tasks,
			chromedp.CaptureScreenshot(&buf),
			chromedp.ActionFunc(func(ctx context.Context) error {
				return os.WriteFile(path, buf, 0644)
			}),
		)

	case "wait":
		if step.Selector != "" {
			tasks = append(tasks,
				chromedp.WaitVisible(step.Selector, chromedp.ByQuery),
			)
		}
		// Additional explicit sleep after wait
		if step.Wait > 0 {
			tasks = append(tasks, chromedp.Sleep(step.Wait))
		}

	case "select":
		if step.Selector == "" || step.Text == "" {
			return fmt.Errorf("select action requires selector and text (option value)")
		}
		tasks = append(tasks,
			chromedp.WaitVisible(step.Selector, chromedp.ByQuery),
			chromedp.SetValue(step.Selector, step.Text, chromedp.ByQuery),
		)

	default:
		return fmt.Errorf("unknown action: %s", step.Action)
	}

	// Add explicit wait after action if specified
	if step.Wait > 0 && step.Action != "wait" {
		tasks = append(tasks, chromedp.Sleep(step.Wait))
	}

	if err := chromedp.Run(ctx, tasks...); err != nil {
		if step.Optional {
			b.logger.Warn("optional step failed", "action", step.Action, "selector", step.Selector, "err", err)
			return nil
		}
		return fmt.Errorf("step '%s' on '%s': %w", step.Action, step.Selector, err)
	}

	return nil
}

// GetPageHTML returns the full HTML content of the current page
func (b *Browser) GetPageHTML() (string, error) {
	var html string
	if err := chromedp.Run(b.ctx,
		chromedp.OuterHTML("html", &html, chromedp.ByQuery),
	); err != nil {
		return "", fmt.Errorf("getting page HTML: %w", err)
	}
	return html, nil
}

// GetText extracts visible text from a CSS selector
func (b *Browser) GetText(selector string) (string, error) {
	var text string
	ctx, cancel := context.WithTimeout(b.ctx, 15*time.Second)
	defer cancel()

	if err := chromedp.Run(ctx,
		chromedp.Text(selector, &text, chromedp.ByQuery),
	); err != nil {
		return "", fmt.Errorf("getting text for '%s': %w", selector, err)
	}
	return text, nil
}

// GetTexts extracts visible text from all matching elements
func (b *Browser) GetTexts(selector string) ([]string, error) {
	var texts []string
	ctx, cancel := context.WithTimeout(b.ctx, 15*time.Second)
	defer cancel()

	if err := chromedp.Run(ctx,
		chromedp.Evaluate(
			fmt.Sprintf(`Array.from(document.querySelectorAll(%q)).map(el => el.innerText.trim())`, selector),
			&texts,
		),
	); err != nil {
		return nil, fmt.Errorf("getting texts for '%s': %w", selector, err)
	}
	return texts, nil
}

// GetAttribute extracts an attribute from a CSS selector
func (b *Browser) GetAttribute(selector, attribute string) (string, error) {
	ctx, cancel := context.WithTimeout(b.ctx, 15*time.Second)
	defer cancel()

	if attribute == "text" || attribute == "innerText" {
		return b.GetText(selector)
	}
	if attribute == "html" || attribute == "innerHTML" {
		var html string
		if err := chromedp.Run(ctx,
			chromedp.InnerHTML(selector, &html, chromedp.ByQuery),
		); err != nil {
			return "", fmt.Errorf("getting innerHTML for '%s': %w", selector, err)
		}
		return html, nil
	}

	var val string
	var ok bool
	if err := chromedp.Run(ctx,
		chromedp.AttributeValue(selector, attribute, &val, &ok, chromedp.ByQuery),
	); err != nil {
		return "", fmt.Errorf("getting attribute '%s' for '%s': %w", attribute, selector, err)
	}
	if !ok {
		return "", nil
	}
	return val, nil
}

// GetAttributes extracts an attribute from all matching elements
func (b *Browser) GetAttributes(selector, attribute string) ([]string, error) {
	ctx, cancel := context.WithTimeout(b.ctx, 15*time.Second)
	defer cancel()

	var vals []string
	if attribute == "text" || attribute == "innerText" {
		return b.GetTexts(selector)
	}

	if err := chromedp.Run(ctx,
		chromedp.Evaluate(
			fmt.Sprintf(`Array.from(document.querySelectorAll(%q)).map(el => el.getAttribute(%q) || '')`, selector, attribute),
			&vals,
		),
	); err != nil {
		return nil, fmt.Errorf("getting attributes '%s' for '%s': %w", attribute, selector, err)
	}
	return vals, nil
}

// EvalXPath evaluates an XPath expression and returns matching text content
func (b *Browser) EvalXPath(xpath string) (string, error) {
	ctx, cancel := context.WithTimeout(b.ctx, 15*time.Second)
	defer cancel()

	var text string
	if err := chromedp.Run(ctx,
		chromedp.Text(xpath, &text, chromedp.BySearch),
	); err != nil {
		return "", fmt.Errorf("xpath '%s': %w", xpath, err)
	}
	return text, nil
}

// Navigate navigates to a URL
func (b *Browser) Navigate(rawURL string) error {
	ctx, cancel := context.WithTimeout(b.ctx, 30*time.Second)
	defer cancel()
	return chromedp.Run(ctx, chromedp.Navigate(rawURL))
}

// CurrentURL returns the current page URL
func (b *Browser) CurrentURL() (string, error) {
	var currentURL string
	if err := chromedp.Run(b.ctx,
		chromedp.Location(&currentURL),
	); err != nil {
		return "", err
	}
	return currentURL, nil
}
