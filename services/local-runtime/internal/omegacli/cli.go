package omegacli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
)

const defaultAPIURL = "http://127.0.0.1:3888"

type CLI struct {
	APIURL string
	Client *http.Client
	Stdout io.Writer
	Stderr io.Writer
}

func Run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	cli := &CLI{Client: http.DefaultClient, Stdout: stdout, Stderr: stderr}
	if err := cli.Run(ctx, args); err != nil {
		fmt.Fprintf(stderr, "omega: %v\n", err)
		return 1
	}
	return 0
}

func (cli *CLI) Run(ctx context.Context, args []string) error {
	global := flag.NewFlagSet("omega", flag.ContinueOnError)
	global.SetOutput(io.Discard)
	apiURL := global.String("api-url", envOr("OMEGA_API_URL", defaultAPIURL), "Omega Local Runtime API URL")
	jsonOutput := global.Bool("json", false, "print raw JSON for supported commands")
	if err := global.Parse(args); err != nil {
		return err
	}
	cli.APIURL = strings.TrimRight(*apiURL, "/")
	if cli.Client == nil {
		cli.Client = http.DefaultClient
	}
	rest := global.Args()
	if len(rest) == 0 {
		cli.printUsage()
		return nil
	}
	switch rest[0] {
	case "help", "-h", "--help":
		cli.printUsage()
		return nil
	case "health":
		return cli.health(ctx, *jsonOutput)
	case "status":
		return cli.status(ctx, *jsonOutput)
	case "logs":
		return cli.logs(ctx, rest[1:], *jsonOutput)
	case "work-items", "workitems":
		return cli.workItems(ctx, rest[1:], *jsonOutput)
	case "attempts":
		return cli.attempts(ctx, rest[1:], *jsonOutput)
	case "checkpoints":
		return cli.checkpoints(ctx, rest[1:], *jsonOutput)
	case "supervisor":
		return cli.supervisor(ctx, rest[1:], *jsonOutput)
	default:
		return fmt.Errorf("unknown command %q", rest[0])
	}
}

func (cli *CLI) printUsage() {
	fmt.Fprintln(cli.Stdout, `Omega CLI

Usage:
  omega [--api-url URL] <command>

Commands:
  health                         Check local runtime health
  status                         Show observability summary
  logs [--level L] [--limit N]   Show runtime logs
  work-items list                List work items
  work-items run <id-or-key>     Run a Work Item through devflow-pr
  attempts list                  List attempts
  attempts timeline <id>         Show an attempt timeline
  attempts retry <id>            Retry a failed/stalled/canceled attempt
  attempts cancel <id>           Cancel a running attempt
  checkpoints list               List checkpoints
  checkpoints approve <id>       Approve a checkpoint
  checkpoints changes <id>       Request checkpoint changes
  supervisor tick                Run one JobSupervisor tick

Supervisor safety:
  supervisor tick only observes/repairs by default. Use --auto-run-ready or
  --auto-retry-failed explicitly before it starts repository-writing jobs.`)
}

func (cli *CLI) health(ctx context.Context, jsonOutput bool) error {
	var body map[string]any
	if err := cli.get(ctx, "/health", &body); err != nil {
		return err
	}
	if jsonOutput {
		return printJSON(cli.Stdout, body)
	}
	fmt.Fprintf(cli.Stdout, "ok=%v implementation=%s persistence=%s\n", body["ok"], text(body, "implementation"), text(body, "persistence"))
	return nil
}

func (cli *CLI) status(ctx context.Context, jsonOutput bool) error {
	var body map[string]any
	if err := cli.get(ctx, "/observability", &body); err != nil {
		return err
	}
	if jsonOutput {
		return printJSON(cli.Stdout, body)
	}
	counts := mapValue(body["counts"])
	attention := mapValue(body["attention"])
	fmt.Fprintf(cli.Stdout, "workItems=%d pipelines=%d attempts=%d checkpoints=%d runtimeLogs=%d\n",
		intValue(counts["workItems"]), intValue(counts["pipelines"]), intValue(counts["attempts"]), intValue(counts["checkpoints"]), intValue(counts["runtimeLogs"]))
	fmt.Fprintf(cli.Stdout, "attention waitingHuman=%d failed=%d blocked=%d\n",
		intValue(attention["waitingHuman"]), intValue(attention["failed"]), intValue(attention["blocked"]))
	dashboard := mapValue(body["dashboard"])
	attempts := mapValue(dashboard["attempts"])
	if len(attempts) > 0 {
		fmt.Fprintf(cli.Stdout, "attempts total=%d active=%d terminal=%d successRate=%.2f\n",
			intValue(attempts["total"]), intValue(attempts["active"]), intValue(attempts["terminal"]), floatValue(attempts["successRate"]))
	}
	if actions := arrayMaps(dashboard["recommendedActions"]); len(actions) > 0 {
		fmt.Fprintln(cli.Stdout, "\nRecommended actions:")
		for _, action := range actions {
			fmt.Fprintf(cli.Stdout, "- %s (%d)\n", text(action, "label"), intValue(action["count"]))
		}
	}
	if errorsList := arrayMaps(body["recentErrors"]); len(errorsList) > 0 {
		fmt.Fprintln(cli.Stdout, "\nRecent errors:")
		for _, entry := range errorsList {
			fmt.Fprintf(cli.Stdout, "- %s %s\n", text(entry, "eventType"), text(entry, "message"))
		}
	}
	return nil
}

func (cli *CLI) logs(ctx context.Context, args []string, jsonOutput bool) error {
	flags := flag.NewFlagSet("logs", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	level := flags.String("level", "", "log level")
	eventType := flags.String("event-type", "", "event type")
	limit := flags.Int("limit", 20, "maximum records")
	if _, err := parseCommandFlags(flags, args); err != nil {
		return err
	}
	query := url.Values{}
	if *level != "" {
		query.Set("level", *level)
	}
	if *eventType != "" {
		query.Set("eventType", *eventType)
	}
	query.Set("limit", strconv.Itoa(*limit))
	var logs []map[string]any
	if err := cli.get(ctx, "/runtime-logs?"+query.Encode(), &logs); err != nil {
		return err
	}
	if jsonOutput {
		return printJSON(cli.Stdout, logs)
	}
	writer := tabwriter.NewWriter(cli.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "TIME\tLEVEL\tEVENT\tMESSAGE")
	for _, entry := range logs {
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", text(entry, "createdAt"), text(entry, "level"), text(entry, "eventType"), oneLine(text(entry, "message"), 96))
	}
	return writer.Flush()
}

func (cli *CLI) workItems(ctx context.Context, args []string, jsonOutput bool) error {
	if len(args) == 0 {
		return errors.New("work-items requires a subcommand: list, run")
	}
	switch args[0] {
	case "list":
		return cli.workItemsList(ctx, args[1:], jsonOutput)
	case "run":
		return cli.workItemsRun(ctx, args[1:], jsonOutput)
	default:
		return fmt.Errorf("unknown work-items subcommand %q", args[0])
	}
}

func (cli *CLI) workItemsList(ctx context.Context, args []string, jsonOutput bool) error {
	flags := flag.NewFlagSet("work-items list", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	status := flags.String("status", "", "filter by status")
	if _, err := parseCommandFlags(flags, args); err != nil {
		return err
	}
	workspace, err := cli.workspace(ctx)
	if err != nil {
		return err
	}
	items := arrayMaps(mapValue(workspace["tables"])["workItems"])
	if *status != "" {
		filtered := []map[string]any{}
		for _, item := range items {
			if strings.EqualFold(text(item, "status"), *status) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	sort.SliceStable(items, func(i, j int) bool {
		return text(items[i], "updatedAt") > text(items[j], "updatedAt")
	})
	if jsonOutput {
		return printJSON(cli.Stdout, items)
	}
	writer := tabwriter.NewWriter(cli.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "KEY\tSTATUS\tREPOSITORY\tTITLE")
	for _, item := range items {
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", stringOr(text(item, "key"), text(item, "id")), text(item, "status"), text(item, "repositoryTargetId"), oneLine(text(item, "title"), 72))
	}
	return writer.Flush()
}

func (cli *CLI) workItemsRun(ctx context.Context, args []string, jsonOutput bool) error {
	flags := flag.NewFlagSet("work-items run", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	wait := flags.Bool("wait", false, "wait for run completion")
	autoApproveHuman := flags.Bool("auto-approve-human", false, "auto-approve human gate")
	autoMerge := flags.Bool("auto-merge", false, "auto-merge after successful review")
	positionals, err := parseCommandFlags(flags, args)
	if err != nil {
		return err
	}
	if len(positionals) != 1 {
		return errors.New("work-items run requires <id-or-key>")
	}
	workspace, err := cli.workspace(ctx)
	if err != nil {
		return err
	}
	item, ok := findWorkItem(workspace, positionals[0])
	if !ok {
		return fmt.Errorf("work item %q not found", positionals[0])
	}
	pipeline, ok := findDevFlowPipeline(workspace, text(item, "id"))
	if !ok {
		var created map[string]any
		if err := cli.post(ctx, "/pipelines/from-template", map[string]any{"item": item, "templateId": "devflow-pr"}, &created); err != nil {
			return err
		}
		pipeline = created
	}
	payload := map[string]any{
		"wait":             *wait,
		"autoApproveHuman": *autoApproveHuman,
		"autoMerge":        *autoMerge,
	}
	var result map[string]any
	if err := cli.post(ctx, "/pipelines/"+url.PathEscape(text(pipeline, "id"))+"/run-devflow-cycle", payload, &result); err != nil {
		return err
	}
	if jsonOutput {
		return printJSON(cli.Stdout, result)
	}
	attempt := mapValue(result["attempt"])
	fmt.Fprintf(cli.Stdout, "status=%s workItem=%s pipeline=%s attempt=%s\n",
		text(result, "status"), stringOr(text(item, "key"), text(item, "id")), text(pipeline, "id"), text(attempt, "id"))
	return nil
}

func (cli *CLI) attempts(ctx context.Context, args []string, jsonOutput bool) error {
	if len(args) == 0 {
		return errors.New("attempts requires a subcommand: list, timeline, retry, cancel")
	}
	switch args[0] {
	case "list":
		return cli.attemptsList(ctx, args[1:], jsonOutput)
	case "timeline":
		return cli.attemptTimeline(ctx, args[1:], jsonOutput)
	case "retry":
		return cli.attemptRetry(ctx, args[1:], jsonOutput)
	case "cancel":
		return cli.attemptCancel(ctx, args[1:], jsonOutput)
	default:
		return fmt.Errorf("unknown attempts subcommand %q", args[0])
	}
}

func (cli *CLI) attemptsList(ctx context.Context, args []string, jsonOutput bool) error {
	flags := flag.NewFlagSet("attempts list", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	status := flags.String("status", "", "filter by status")
	if _, err := parseCommandFlags(flags, args); err != nil {
		return err
	}
	var attempts []map[string]any
	if err := cli.get(ctx, "/attempts", &attempts); err != nil {
		return err
	}
	if *status != "" {
		filtered := []map[string]any{}
		for _, attempt := range attempts {
			if strings.EqualFold(text(attempt, "status"), *status) {
				filtered = append(filtered, attempt)
			}
		}
		attempts = filtered
	}
	if jsonOutput {
		return printJSON(cli.Stdout, attempts)
	}
	writer := tabwriter.NewWriter(cli.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "ID\tSTATUS\tITEM\tSTAGE\tUPDATED")
	for _, attempt := range attempts {
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n", text(attempt, "id"), text(attempt, "status"), text(attempt, "itemId"), text(attempt, "currentStageId"), text(attempt, "updatedAt"))
	}
	return writer.Flush()
}

func (cli *CLI) attemptTimeline(ctx context.Context, args []string, jsonOutput bool) error {
	if len(args) != 1 {
		return errors.New("attempts timeline requires <attempt-id>")
	}
	var timeline map[string]any
	if err := cli.get(ctx, "/attempts/"+url.PathEscape(args[0])+"/timeline", &timeline); err != nil {
		return err
	}
	if jsonOutput {
		return printJSON(cli.Stdout, timeline)
	}
	items := arrayMaps(timeline["items"])
	writer := tabwriter.NewWriter(cli.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "TIME\tTYPE\tSTATUS\tSUMMARY")
	for _, item := range items {
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", text(item, "createdAt"), text(item, "type"), text(item, "status"), oneLine(text(item, "summary"), 96))
	}
	return writer.Flush()
}

func (cli *CLI) attemptRetry(ctx context.Context, args []string, jsonOutput bool) error {
	flags := flag.NewFlagSet("attempts retry", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	reason := flags.String("reason", "Retried from Omega CLI.", "retry reason")
	positionals, err := parseCommandFlags(flags, args)
	if err != nil {
		return err
	}
	if len(positionals) != 1 {
		return errors.New("attempts retry requires <attempt-id>")
	}
	var result map[string]any
	if err := cli.post(ctx, "/attempts/"+url.PathEscape(positionals[0])+"/retry", map[string]any{"reason": *reason}, &result); err != nil {
		return err
	}
	if jsonOutput {
		return printJSON(cli.Stdout, result)
	}
	attempt := mapValue(result["attempt"])
	fmt.Fprintf(cli.Stdout, "retryAttempt=%s status=%s\n", text(attempt, "id"), text(attempt, "status"))
	return nil
}

func (cli *CLI) attemptCancel(ctx context.Context, args []string, jsonOutput bool) error {
	flags := flag.NewFlagSet("attempts cancel", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	reason := flags.String("reason", "Canceled from Omega CLI.", "cancel reason")
	positionals, err := parseCommandFlags(flags, args)
	if err != nil {
		return err
	}
	if len(positionals) != 1 {
		return errors.New("attempts cancel requires <attempt-id>")
	}
	var result map[string]any
	if err := cli.post(ctx, "/attempts/"+url.PathEscape(positionals[0])+"/cancel", map[string]any{"reason": *reason}, &result); err != nil {
		return err
	}
	if jsonOutput {
		return printJSON(cli.Stdout, result)
	}
	fmt.Fprintf(cli.Stdout, "attempt=%s status=%s cancelSignalSent=%v\n", positionals[0], text(result, "status"), result["cancelSignalSent"])
	return nil
}

func (cli *CLI) checkpoints(ctx context.Context, args []string, jsonOutput bool) error {
	if len(args) == 0 {
		return errors.New("checkpoints requires a subcommand: list, approve, changes")
	}
	switch args[0] {
	case "list":
		return cli.checkpointsList(ctx, args[1:], jsonOutput)
	case "approve":
		return cli.checkpointApprove(ctx, args[1:], jsonOutput)
	case "changes", "request-changes":
		return cli.checkpointChanges(ctx, args[1:], jsonOutput)
	default:
		return fmt.Errorf("unknown checkpoints subcommand %q", args[0])
	}
}

func (cli *CLI) checkpointsList(ctx context.Context, args []string, jsonOutput bool) error {
	flags := flag.NewFlagSet("checkpoints list", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	status := flags.String("status", "", "filter by status")
	if _, err := parseCommandFlags(flags, args); err != nil {
		return err
	}
	var checkpoints []map[string]any
	if err := cli.get(ctx, "/checkpoints", &checkpoints); err != nil {
		return err
	}
	if *status != "" {
		filtered := []map[string]any{}
		for _, checkpoint := range checkpoints {
			if strings.EqualFold(text(checkpoint, "status"), *status) {
				filtered = append(filtered, checkpoint)
			}
		}
		checkpoints = filtered
	}
	if jsonOutput {
		return printJSON(cli.Stdout, checkpoints)
	}
	writer := tabwriter.NewWriter(cli.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "ID\tSTATUS\tSTAGE\tATTEMPT\tTITLE")
	for _, checkpoint := range checkpoints {
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n", text(checkpoint, "id"), text(checkpoint, "status"), text(checkpoint, "stageId"), text(checkpoint, "attemptId"), text(checkpoint, "title"))
	}
	return writer.Flush()
}

func (cli *CLI) checkpointApprove(ctx context.Context, args []string, jsonOutput bool) error {
	flags := flag.NewFlagSet("checkpoints approve", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	reviewer := flags.String("reviewer", envOr("USER", "omega-cli"), "reviewer name")
	positionals, err := parseCommandFlags(flags, args)
	if err != nil {
		return err
	}
	if len(positionals) != 1 {
		return errors.New("checkpoints approve requires <checkpoint-id>")
	}
	var result map[string]any
	if err := cli.post(ctx, "/checkpoints/"+url.PathEscape(positionals[0])+"/approve", map[string]any{"reviewer": *reviewer}, &result); err != nil {
		return err
	}
	if jsonOutput {
		return printJSON(cli.Stdout, result)
	}
	fmt.Fprintf(cli.Stdout, "checkpoint=%s status=%s\n", positionals[0], text(result, "status"))
	return nil
}

func (cli *CLI) checkpointChanges(ctx context.Context, args []string, jsonOutput bool) error {
	flags := flag.NewFlagSet("checkpoints changes", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	reason := flags.String("reason", "", "change request reason")
	positionals, err := parseCommandFlags(flags, args)
	if err != nil {
		return err
	}
	if len(positionals) != 1 {
		return errors.New("checkpoints changes requires <checkpoint-id>")
	}
	if strings.TrimSpace(*reason) == "" {
		return errors.New("checkpoints changes requires --reason")
	}
	var result map[string]any
	if err := cli.post(ctx, "/checkpoints/"+url.PathEscape(positionals[0])+"/request-changes", map[string]any{"reason": *reason}, &result); err != nil {
		return err
	}
	if jsonOutput {
		return printJSON(cli.Stdout, result)
	}
	fmt.Fprintf(cli.Stdout, "checkpoint=%s status=%s\n", positionals[0], text(result, "status"))
	return nil
}

func (cli *CLI) supervisor(ctx context.Context, args []string, jsonOutput bool) error {
	if len(args) == 0 || args[0] != "tick" {
		return errors.New("supervisor requires subcommand: tick")
	}
	flags := flag.NewFlagSet("supervisor tick", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	autoRunReady := flags.Bool("auto-run-ready", false, "start runnable Ready work items")
	autoRetryFailed := flags.Bool("auto-retry-failed", false, "retry failed or stalled attempts")
	staleAfterSeconds := flags.Int("stale-after-seconds", 0, "stalled threshold in seconds")
	retryBackoffSeconds := flags.Int("retry-backoff-seconds", 0, "retry backoff threshold in seconds")
	maxRetryAttempts := flags.Int("max-retry-attempts", 2, "maximum retries per attempt root")
	limit := flags.Int("limit", 10, "Ready item scan limit")
	if _, err := parseCommandFlags(flags, args[1:]); err != nil {
		return err
	}
	payload := map[string]any{"autoRunReady": *autoRunReady, "autoRetryFailed": *autoRetryFailed, "maxRetryAttempts": *maxRetryAttempts, "limit": *limit}
	if *staleAfterSeconds > 0 {
		payload["staleAfterSeconds"] = *staleAfterSeconds
	}
	if *retryBackoffSeconds > 0 {
		payload["retryBackoffSeconds"] = *retryBackoffSeconds
	}
	var summary map[string]any
	if err := cli.post(ctx, "/job-supervisor/tick", payload, &summary); err != nil {
		return err
	}
	if jsonOutput {
		return printJSON(cli.Stdout, summary)
	}
	fmt.Fprintf(cli.Stdout, "changed=%d pendingCheckpoints=%d stalledAttempts=%d runnableItems=%d acceptedReadyRuns=%d\n",
		intValue(summary["changed"]), intValue(summary["pendingCheckpoints"]), intValue(summary["stalledAttempts"]), intValue(summary["runnableItems"]), intValue(summary["acceptedReadyRuns"]))
	if intValue(summary["checkedRecoveryAttempts"]) > 0 || intValue(summary["acceptedRetryAttempts"]) > 0 {
		fmt.Fprintf(cli.Stdout, "recovery retryable=%d acceptedRetries=%d backoff=%d limitReached=%d\n",
			intValue(summary["retryableAttempts"]), intValue(summary["acceptedRetryAttempts"]), intValue(summary["retryBackoff"]), intValue(summary["retryLimitReached"]))
	}
	return nil
}

func (cli *CLI) workspace(ctx context.Context) (map[string]any, error) {
	var workspace map[string]any
	if err := cli.get(ctx, "/workspace", &workspace); err != nil {
		return nil, err
	}
	return workspace, nil
}

func (cli *CLI) get(ctx context.Context, path string, out any) error {
	return cli.request(ctx, http.MethodGet, path, nil, out)
}

func (cli *CLI) post(ctx context.Context, path string, payload any, out any) error {
	return cli.request(ctx, http.MethodPost, path, payload, out)
}

func (cli *CLI) request(ctx context.Context, method string, path string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, cli.APIURL+path, body)
	if err != nil {
		return err
	}
	if payload != nil {
		request.Header.Set("content-type", "application/json")
	}
	response, err := cli.Client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var errorBody map[string]any
		if json.Unmarshal(raw, &errorBody) == nil && text(errorBody, "error") != "" {
			return fmt.Errorf("%s %s failed: %s", method, path, text(errorBody, "error"))
		}
		return fmt.Errorf("%s %s failed: %s", method, path, strings.TrimSpace(string(raw)))
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func findWorkItem(workspace map[string]any, idOrKey string) (map[string]any, bool) {
	for _, item := range arrayMaps(mapValue(workspace["tables"])["workItems"]) {
		if text(item, "id") == idOrKey || strings.EqualFold(text(item, "key"), idOrKey) {
			return item, true
		}
	}
	return nil, false
}

func findDevFlowPipeline(workspace map[string]any, itemID string) (map[string]any, bool) {
	for _, pipeline := range arrayMaps(mapValue(workspace["tables"])["pipelines"]) {
		if text(pipeline, "workItemId") == itemID && text(pipeline, "templateId") == "devflow-pr" {
			return pipeline, true
		}
	}
	return nil, false
}

func printJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func parseCommandFlags(flags *flag.FlagSet, args []string) ([]string, error) {
	flagArgs := []string{}
	positionals := []string{}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--" {
			positionals = append(positionals, args[index+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positionals = append(positionals, arg)
			continue
		}
		flagArgs = append(flagArgs, arg)
		flagName := strings.TrimLeft(arg, "-")
		if before, _, ok := strings.Cut(flagName, "="); ok {
			flagName = before
		}
		if current := flags.Lookup(flagName); current != nil {
			if getter, ok := current.Value.(flag.Getter); ok {
				if _, isBool := getter.Get().(bool); isBool {
					continue
				}
			}
		}
		if strings.Contains(arg, "=") {
			continue
		}
		if index+1 < len(args) {
			index++
			flagArgs = append(flagArgs, args[index])
		}
	}
	if err := flags.Parse(flagArgs); err != nil {
		return nil, err
	}
	return positionals, nil
}

func envOr(name string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func text(source map[string]any, key string) string {
	if source == nil {
		return ""
	}
	switch value := source[key].(type) {
	case string:
		return value
	case fmt.Stringer:
		return value.String()
	case int:
		return strconv.Itoa(value)
	case int64:
		return strconv.FormatInt(value, 10)
	case float64:
		return strconv.Itoa(int(value))
	case bool:
		return strconv.FormatBool(value)
	default:
		return ""
	}
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		next, _ := typed.Int64()
		return int(next)
	case string:
		next, _ := strconv.Atoi(typed)
		return next
	default:
		return 0
	}
}

func floatValue(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		next, _ := typed.Float64()
		return next
	case string:
		next, _ := strconv.ParseFloat(typed, 64)
		return next
	default:
		return 0
	}
}

func stringOr(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func mapValue(value any) map[string]any {
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return map[string]any{}
}

func arrayMaps(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		return typed
	case []any:
		items := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if mapped, ok := item.(map[string]any); ok {
				items = append(items, mapped)
			}
		}
		return items
	default:
		return []map[string]any{}
	}
}

func oneLine(value string, max int) string {
	value = strings.Join(strings.Fields(value), " ")
	if max > 0 && len(value) > max {
		if max <= 3 {
			return value[:max]
		}
		return value[:max-3] + "..."
	}
	return value
}
