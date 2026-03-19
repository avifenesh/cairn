package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/avifenesh/cairn/internal/tool"
)

// gwsConfig holds the resolved path to the gws CLI binary.
var gwsConfig struct {
	path    string
	enabled bool
}

// SetGWSConfig configures the Google Workspace CLI tools.
func SetGWSConfig(gwsPath string) {
	gwsConfig.path = gwsPath
	gwsConfig.enabled = gwsPath != ""
}

// GWSEnabled returns true if the gws CLI is available.
func GWSEnabled() bool {
	return gwsConfig.enabled
}

// Allowed services for gws tools — all 17 services supported by the gws CLI.
var gwsServices = map[string]bool{
	"drive": true, "sheets": true, "gmail": true, "calendar": true,
	"docs": true, "slides": true, "tasks": true, "people": true,
	"chat": true, "classroom": true, "forms": true, "keep": true,
	"meet": true, "events": true, "admin-reports": true,
	"modelarmor": true, "workflow": true,
}

// Read-only methods that gws.query allows.
var gwsReadMethods = map[string]bool{
	"list": true, "get": true, "watch": true, "export": true,
	"batchGet": true, "batchGetByDataFilter": true, "search": true,
	"getProfile": true, "getByDataFilter": true, "query": true,
}

// Gmail methods that require approval (external write boundary).
var gwsGmailApprovalMethods = map[string]bool{
	"send": true, "delete": true, "trash": true, "batchDelete": true,
}

func callGWS(ctx context.Context, service, resource, subResource, method string, params, body map[string]any) (string, error) {
	args := []string{service, resource}
	if subResource != "" {
		args = append(args, subResource)
	}
	args = append(args, method)

	if len(params) > 0 {
		p, _ := json.Marshal(params)
		args = append(args, "--params", string(p))
	}
	if len(body) > 0 {
		b, _ := json.Marshal(body)
		args = append(args, "--json", string(b))
	}
	args = append(args, "--format", "json")

	execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(execCtx, gwsConfig.path, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	out, err := cmd.CombinedOutput()
	if execCtx.Err() != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		return "", fmt.Errorf("gws: timeout after 30s")
	}

	output := strings.TrimSpace(string(out))

	if err != nil {
		// Try to extract error message from JSON output.
		var errResp struct {
			Error struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(output), &errResp) == nil && errResp.Error.Message != "" {
			return "", fmt.Errorf("gws: %d %s", errResp.Error.Code, errResp.Error.Message)
		}
		return "", fmt.Errorf("gws: %v", err)
	}

	return output, nil
}

// --- Tool definitions ---

type gwsQueryParams struct {
	Service     string         `json:"service" desc:"Google service: drive, sheets, gmail, calendar, docs, slides, tasks, people, chat, classroom, forms, keep, meet, events, admin-reports, modelarmor, workflow"`
	Resource    string         `json:"resource" desc:"API resource (e.g. files, users, events, spreadsheets, presentations, documents)"`
	Method      string         `json:"method" desc:"Read method: list, get, search, export, batchGet"`
	SubResource string         `json:"subResource,omitempty" desc:"Sub-resource (e.g. messages under users, values under spreadsheets)"`
	Params      map[string]any `json:"params,omitempty" desc:"Query/URL parameters as JSON object"`
}

type gwsExecuteParams struct {
	Service     string         `json:"service" desc:"Google service: drive, sheets, gmail, calendar, docs, slides, tasks, people, chat, classroom, forms, keep, meet, events, admin-reports, modelarmor, workflow"`
	Resource    string         `json:"resource" desc:"API resource (e.g. files, users, events, spreadsheets)"`
	Method      string         `json:"method" desc:"Write method: create, update, patch, delete, send, insert, move, copy, batchUpdate, append, clear"`
	SubResource string         `json:"subResource,omitempty" desc:"Sub-resource (e.g. messages under users, values under spreadsheets)"`
	Params      map[string]any `json:"params,omitempty" desc:"Query/URL parameters"`
	Body        map[string]any `json:"body,omitempty" desc:"Request body as JSON object"`
}

var gwsQuery = tool.Define("cairn.gwsQuery",
	"Query Google Workspace services (read-only). Supports 17 services: drive, sheets, gmail, calendar, "+
		"docs, slides, tasks, people, chat, classroom, forms, keep, meet, events, admin-reports, modelarmor, workflow.\n\n"+
		"Examples:\n"+
		"- List emails: service=gmail, resource=users, subResource=messages, method=list, params={\"userId\":\"me\",\"maxResults\":5}\n"+
		"- List Drive files: service=drive, resource=files, method=list, params={\"pageSize\":10}\n"+
		"- List calendar events: service=calendar, resource=events, method=list, params={\"calendarId\":\"primary\",\"maxResults\":5}\n"+
		"- Get spreadsheet data: service=sheets, resource=spreadsheets, subResource=values, method=get, params={\"spreadsheetId\":\"<id>\",\"range\":\"Sheet1!A1:D10\"}\n"+
		"- Get a doc: service=docs, resource=documents, method=get, params={\"documentId\":\"<id>\"}\n"+
		"- Get a presentation: service=slides, resource=presentations, method=get, params={\"presentationId\":\"<id>\"}\n"+
		"- List task lists: service=tasks, resource=tasklists, method=list\n"+
		"- List Keep notes: service=keep, resource=notes, method=list\n"+
		"- List Chat spaces: service=chat, resource=spaces, method=list\n\n"+
		"Use cairn.gwsExecute for write operations.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork},
	func(ctx *tool.ToolContext, p gwsQueryParams) (*tool.ToolResult, error) {
		if !gwsServices[p.Service] {
			return &tool.ToolResult{Error: fmt.Sprintf("unsupported service: %s", p.Service)}, nil
		}
		if p.Resource == "" || p.Method == "" {
			return &tool.ToolResult{Error: "resource and method are required"}, nil
		}
		if !gwsReadMethods[p.Method] {
			return &tool.ToolResult{Error: fmt.Sprintf("method %q is not a read method — use cairn.gwsExecute", p.Method)}, nil
		}

		output, err := callGWS(safeCtx(ctx.Cancel), p.Service, p.Resource, p.SubResource, p.Method, p.Params, nil)
		if err != nil {
			return &tool.ToolResult{Error: err.Error()}, nil
		}

		return &tool.ToolResult{
			Output:   output,
			Metadata: map[string]any{"provider": "gws", "service": p.Service},
		}, nil
	},
)

var gwsExecute = tool.Define("cairn.gwsExecute",
	"Execute write operations on Google Workspace services. Supports 17 services: drive, sheets, gmail, calendar, "+
		"docs, slides, tasks, people, chat, classroom, forms, keep, meet, events, admin-reports, modelarmor, workflow.\n\n"+
		"Gmail send/delete requires approval. Other write operations execute directly.\n\n"+
		"Examples:\n"+
		"- Send email: service=gmail, resource=users, subResource=messages, method=send, params={\"userId\":\"me\"}, body={...}\n"+
		"- Create event: service=calendar, resource=events, method=insert, params={\"calendarId\":\"primary\"}, body={...}\n"+
		"- Create Drive file: service=drive, resource=files, method=create, body={\"name\":\"test.txt\"}\n"+
		"- Update spreadsheet: service=sheets, resource=spreadsheets, subResource=values, method=update, params={\"spreadsheetId\":\"<id>\",\"range\":\"Sheet1!A1\"}, body={\"values\":[[\"hello\"]]}\n"+
		"- Update a doc: service=docs, resource=documents, method=batchUpdate, params={\"documentId\":\"<id>\"}, body={\"requests\":[...]}\n"+
		"- Create task: service=tasks, resource=tasks, method=insert, params={\"tasklist\":\"<id>\"}, body={\"title\":\"My task\"}\n"+
		"- Send Chat message: service=chat, resource=spaces, subResource=messages, method=create, params={\"parent\":\"spaces/<id>\"}, body={\"text\":\"hello\"}\n\n"+
		"Use cairn.gwsQuery for read operations.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork},
	func(ctx *tool.ToolContext, p gwsExecuteParams) (*tool.ToolResult, error) {
		if !gwsServices[p.Service] {
			return &tool.ToolResult{Error: fmt.Sprintf("unsupported service: %s", p.Service)}, nil
		}
		if p.Resource == "" || p.Method == "" {
			return &tool.ToolResult{Error: "resource and method are required"}, nil
		}
		if gwsReadMethods[p.Method] {
			return &tool.ToolResult{Error: fmt.Sprintf("method %q is a read method — use cairn.gwsQuery", p.Method)}, nil
		}

		// Gmail send/delete — flag as needing approval (agent will surface this).
		if p.Service == "gmail" && gwsGmailApprovalMethods[p.Method] {
			return &tool.ToolResult{
				Error: fmt.Sprintf("Gmail %s requires user approval. Please confirm before proceeding.", p.Method),
				Metadata: map[string]any{
					"requiresApproval": true,
					"action":           fmt.Sprintf("gmail.%s.%s", p.Resource, p.Method),
				},
			}, nil
		}

		output, err := callGWS(safeCtx(ctx.Cancel), p.Service, p.Resource, p.SubResource, p.Method, p.Params, p.Body)
		if err != nil {
			return &tool.ToolResult{Error: err.Error()}, nil
		}

		return &tool.ToolResult{
			Output:   output,
			Metadata: map[string]any{"provider": "gws", "service": p.Service},
		}, nil
	},
)
