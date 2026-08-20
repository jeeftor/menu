package server

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

const lambdaFunctionName = "menu-alexa"
const lambdaRegion = "us-east-1"

// handleAlexaStats renders a page showing Lambda invocation metrics from CloudWatch.
func (s *Server) handleAlexaStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// Check for AWS credentials
	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	if accessKey == "" || secretKey == "" {
		fmt.Fprint(w, strings.ReplaceAll(alexaStatsPage, "[[STATS_CONTENT]]",
			`<p class="warn">AWS credentials not configured. Set <code>AWS_ACCESS_KEY_ID</code> and <code>AWS_SECRET_ACCESS_KEY</code> environment variables to enable Lambda metrics.</p>`))
		return
	}

	stats, err := fetchLambdaMetrics(r.Context())
	if err != nil {
		slog.Error("failed to fetch lambda metrics", "error", err)
		fmt.Fprint(w, strings.ReplaceAll(alexaStatsPage, "[[STATS_CONTENT]]",
			fmt.Sprintf(`<p class="warn">Failed to fetch Lambda metrics: %s</p>`, html.EscapeString(err.Error()))))
		return
	}

	content := renderStatsContent(stats)
	fmt.Fprint(w, strings.ReplaceAll(alexaStatsPage, "[[STATS_CONTENT]]", content))
}

// lambdaStats holds the metrics we display.
type lambdaStats struct {
	TotalInvocations int64
	TotalErrors      int64
	TotalThrottles   int64
	AvgDuration      float64 // milliseconds
	MaxDuration      float64 // milliseconds
	LastInvoked      string
	DailyInvocations []dailyInvocation
}

type dailyInvocation struct {
	Date  string
	Count int64
}

func fetchLambdaMetrics(ctx context.Context) (*lambdaStats, error) {
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(lambdaRegion),
	)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}

	cw := cloudwatch.NewFromConfig(cfg)
	now := time.Now().UTC()
	start := now.AddDate(0, 0, -30) // last 30 days

	dim := []types.Dimension{
		{Name: aws.String("FunctionName"), Value: aws.String(lambdaFunctionName)},
	}

	stats := &lambdaStats{}

	// Invocations (sum over 30 days)
	invocations, err := getMetricStats(ctx, cw, "AWS/Lambda", "Invocations", dim, start, now, types.StatisticSum)
	if err != nil {
		return nil, fmt.Errorf("fetching invocations: %w", err)
	}
	for _, dp := range invocations {
		stats.TotalInvocations += int64(dp)
	}

	// Errors
	errors, err := getMetricStats(ctx, cw, "AWS/Lambda", "Errors", dim, start, now, types.StatisticSum)
	if err != nil {
		return nil, fmt.Errorf("fetching errors: %w", err)
	}
	for _, dp := range errors {
		stats.TotalErrors += int64(dp)
	}

	// Throttles
	throttles, err := getMetricStats(ctx, cw, "AWS/Lambda", "Throttles", dim, start, now, types.StatisticSum)
	if err != nil {
		return nil, fmt.Errorf("fetching throttles: %w", err)
	}
	for _, dp := range throttles {
		stats.TotalThrottles += int64(dp)
	}

	// Duration (average and max)
	durAvg, err := getMetricStats(ctx, cw, "AWS/Lambda", "Duration", dim, start, now, types.StatisticAverage)
	if err != nil {
		return nil, fmt.Errorf("fetching duration avg: %w", err)
	}
	if len(durAvg) > 0 {
		var sum float64
		for _, v := range durAvg {
			sum += v
		}
		stats.AvgDuration = sum / float64(len(durAvg))
	}

	durMax, err := getMetricStats(ctx, cw, "AWS/Lambda", "Duration", dim, start, now, types.StatisticMaximum)
	if err != nil {
		return nil, fmt.Errorf("fetching duration max: %w", err)
	}
	for _, v := range durMax {
		if v > stats.MaxDuration {
			stats.MaxDuration = v
		}
	}

	// Daily invocations for the bar chart
	daily, err := getMetricDaily(ctx, cw, "AWS/Lambda", "Invocations", dim, start, now)
	if err != nil {
		return nil, fmt.Errorf("fetching daily invocations: %w", err)
	}
	stats.DailyInvocations = daily

	// Find last invocation time
	if len(daily) > 0 {
		last := daily[len(daily)-1]
		if last.Count > 0 {
			stats.LastInvoked = last.Date
		} else {
			// Find the last day with invocations
			for i := len(daily) - 1; i >= 0; i-- {
				if daily[i].Count > 0 {
					stats.LastInvoked = daily[i].Date
					break
				}
			}
		}
	}
	if stats.LastInvoked == "" {
		stats.LastInvoked = "Never"
	}

	return stats, nil
}

func getMetricStats(ctx context.Context, cw *cloudwatch.Client, namespace, metric string, dims []types.Dimension, start, end time.Time, stat types.Statistic) ([]float64, error) {
	resp, err := cw.GetMetricStatistics(ctx, &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String(namespace),
		MetricName: aws.String(metric),
		Dimensions: dims,
		StartTime:  aws.Time(start),
		EndTime:    aws.Time(end),
		Period:     aws.Int32(86400), // daily
		Statistics: []types.Statistic{stat},
	})
	if err != nil {
		return nil, err
	}

	var values []float64
	for _, dp := range resp.Datapoints {
		var v float64
		switch stat {
		case types.StatisticSum:
			v = aws.ToFloat64(dp.Sum)
		case types.StatisticAverage:
			v = aws.ToFloat64(dp.Average)
		case types.StatisticMaximum:
			v = aws.ToFloat64(dp.Maximum)
		}
		values = append(values, v)
	}
	return values, nil
}

func getMetricDaily(ctx context.Context, cw *cloudwatch.Client, namespace, metric string, dims []types.Dimension, start, end time.Time) ([]dailyInvocation, error) {
	resp, err := cw.GetMetricStatistics(ctx, &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String(namespace),
		MetricName: aws.String(metric),
		Dimensions: dims,
		StartTime:  aws.Time(start),
		EndTime:    aws.Time(end),
		Period:     aws.Int32(86400),
		Statistics: []types.Statistic{types.StatisticSum},
	})
	if err != nil {
		return nil, err
	}

	// Build a map of date -> count
	countByDate := make(map[string]int64)
	for _, dp := range resp.Datapoints {
		date := dp.Timestamp.UTC().Format("2006-01-02")
		countByDate[date] = int64(aws.ToFloat64(dp.Sum))
	}

	// Build continuous list
	var result []dailyInvocation
	for d := start; d.Before(end); d = d.AddDate(0, 0, 1) {
		dateStr := d.Format("2006-01-02")
		result = append(result, dailyInvocation{
			Date:  dateStr,
			Count: countByDate[dateStr],
		})
	}
	return result, nil
}

func renderStatsContent(stats *lambdaStats) string {
	var sb strings.Builder

	errorRate := 0.0
	if stats.TotalInvocations > 0 {
		errorRate = float64(stats.TotalErrors) / float64(stats.TotalInvocations) * 100
	}

	sb.WriteString(`<div class="stats-grid">`)
	sb.WriteString(fmt.Sprintf(`<div class="stat-card"><div class="stat-value">%d</div><div class="stat-label">Total Invocations (30d)</div></div>`, stats.TotalInvocations))
	sb.WriteString(fmt.Sprintf(`<div class="stat-card"><div class="stat-value">%d</div><div class="stat-label">Errors (30d)</div></div>`, stats.TotalErrors))
	sb.WriteString(fmt.Sprintf(`<div class="stat-card"><div class="stat-value">%.1f%%</div><div class="stat-label">Error Rate</div></div>`, errorRate))
	sb.WriteString(fmt.Sprintf(`<div class="stat-card"><div class="stat-value">%.0fms</div><div class="stat-label">Avg Duration</div></div>`, stats.AvgDuration))
	sb.WriteString(fmt.Sprintf(`<div class="stat-card"><div class="stat-value">%.0fms</div><div class="stat-label">Max Duration</div></div>`, stats.MaxDuration))
	sb.WriteString(fmt.Sprintf(`<div class="stat-card"><div class="stat-value">%s</div><div class="stat-label">Last Invoked</div></div>`, stats.LastInvoked))
	sb.WriteString(`</div>`)

	// Daily invocations bar chart
	if len(stats.DailyInvocations) > 0 {
		sb.WriteString(`<h3>Daily Invocations (last 30 days)</h3>`)
		sb.WriteString(`<div class="bar-chart">`)
		var maxCount int64
		for _, d := range stats.DailyInvocations {
			if d.Count > maxCount {
				maxCount = d.Count
			}
		}
		for _, d := range stats.DailyInvocations {
			height := 0
			if maxCount > 0 {
				height = int(float64(d.Count) / float64(maxCount) * 100)
			}
			label := ""
			if d.Count > 0 {
				label = fmt.Sprintf(`<span class="bar-label">%d</span>`, d.Count)
			}
			class := "bar"
			if d.Count == 0 {
				class = "bar empty"
			}
			sb.WriteString(fmt.Sprintf(`<div class="bar-day"><div class="%s" style="height: %d%%" title="%s: %d">%s</div><span class="day-label">%s</span></div>`,
				class, height, d.Date, d.Count, label, d.Date[5:]))
		}
		sb.WriteString(`</div>`)
	}

	return sb.String()
}

const alexaStatsPage = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Alexa Lambda Stats &mdash; Menu</title>
<style>
  :root {
    --bg: #1a1a2e; --card: #16213e; --text: #e0e0e0; --accent: #0f3460;
    --border: #233; --green: #4caf50; --red: #f44336; --blue: #2196f3;
  }
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { font-family: -apple-system, system-ui, sans-serif; background: var(--bg); color: var(--text); padding: 2rem; }
  h1 { margin-bottom: 0.5rem; }
  h3 { margin: 2rem 0 1rem; }
  a { color: var(--blue); text-decoration: none; }
  a:hover { text-decoration: underline; }
  .nav { margin-bottom: 2rem; font-size: 0.9rem; }
  .nav a { margin-right: 1rem; }
  .stats-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(180px, 1fr)); gap: 1rem; margin-bottom: 2rem; }
  .stat-card { background: var(--card); border: 1px solid var(--border); border-radius: 8px; padding: 1.5rem; text-align: center; }
  .stat-value { font-size: 2rem; font-weight: bold; color: var(--green); }
  .stat-label { font-size: 0.85rem; color: #888; margin-top: 0.5rem; }
  .bar-chart { display: flex; align-items: flex-end; gap: 2px; height: 150px; background: var(--card); border: 1px solid var(--border); border-radius: 8px; padding: 1rem; overflow-x: auto; }
  .bar-day { display: flex; flex-direction: column; align-items: center; justify-content: flex-end; min-width: 28px; height: 100%; position: relative; }
  .bar { width: 20px; background: var(--blue); border-radius: 3px 3px 0 0; min-height: 2px; position: relative; transition: height 0.3s; }
  .bar.empty { background: var(--border); min-height: 2px; }
  .bar-label { position: absolute; top: -18px; font-size: 0.7rem; color: var(--text); white-space: nowrap; }
  .day-label { font-size: 0.6rem; color: #666; margin-top: 4px; writing-mode: vertical-rl; transform: rotate(180deg); }
  .warn { background: #332; color: #cc9; padding: 1rem; border-radius: 8px; border: 1px solid #665; }
  code { background: var(--accent); padding: 2px 6px; border-radius: 3px; font-size: 0.9em; }
</style>
</head>
<body>
  <h1>Alexa Lambda Stats</h1>
  <div class="nav">
    <a href="/calendar">Calendar</a>
    <a href="/settings">Settings</a>
    <a href="/api">API</a>
    <a href="/alexa-stats">Alexa Stats</a>
  </div>
  [[STATS_CONTENT]]
</body>
</html>`
