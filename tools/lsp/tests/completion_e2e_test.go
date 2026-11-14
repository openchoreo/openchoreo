// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp_test

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

type jsonrpcMessage struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      *int        `json:"id,omitempty"`
	Method  string      `json:"method,omitempty"`
	Params  interface{} `json:"params,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   interface{} `json:"error,omitempty"`
}

type completionItem struct {
	Label      string `json:"label"`
	Kind       int    `json:"kind,omitempty"`
	InsertText string `json:"insertText,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

type completionList struct {
	IsIncomplete bool             `json:"isIncomplete"`
	Items        []completionItem `json:"items"`
}

func sendRequest(writer *bufio.Writer, id int, method string, params interface{}) error {
	msg := jsonrpcMessage{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  method,
		Params:  params,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	if _, err := writer.WriteString(header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	if _, err := writer.Write(data); err != nil {
		return fmt.Errorf("write body: %w", err)
	}
	return writer.Flush()
}

func sendNotification(writer *bufio.Writer, method string, params interface{}) error {
	msg := jsonrpcMessage{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	if _, err := writer.WriteString(header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	if _, err := writer.Write(data); err != nil {
		return fmt.Errorf("write body: %w", err)
	}
	return writer.Flush()
}

func readResponse(reader *bufio.Reader) (*jsonrpcMessage, error) {
	var contentLength int
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("read header line: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length: ") {
			val := strings.TrimPrefix(line, "Content-Length: ")
			contentLength, err = strconv.Atoi(val)
			if err != nil {
				return nil, fmt.Errorf("parse Content-Length: %w", err)
			}
		}
	}
	if contentLength <= 0 {
		return nil, fmt.Errorf("invalid Content-Length: %d", contentLength)
	}
	body := make([]byte, contentLength)
	_, err := io.ReadFull(reader, body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	var msg jsonrpcMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	return &msg, nil
}

func readUntilResponse(reader *bufio.Reader, expectedID int) (*jsonrpcMessage, error) {
	for {
		msg, err := readResponse(reader)
		if err != nil {
			return nil, err
		}
		if msg.ID != nil && *msg.ID == expectedID {
			return msg, nil
		}
		if msg.Method != "" {
			continue
		}
	}
}

func completionKindString(kind int) string {
	switch kind {
	case 1:
		return "Text"
	case 2:
		return "Method"
	case 3:
		return "Function"
	case 4:
		return "Constructor"
	case 5:
		return "Field"
	case 6:
		return "Variable"
	case 7:
		return "Class"
	case 8:
		return "Interface"
	case 9:
		return "Module"
	case 10:
		return "Property"
	case 11:
		return "Unit"
	case 12:
		return "Value"
	case 13:
		return "Enum"
	case 14:
		return "Keyword"
	case 15:
		return "Snippet"
	case 16:
		return "Color"
	case 17:
		return "File"
	case 18:
		return "Reference"
	case 19:
		return "Folder"
	case 20:
		return "EnumMember"
	case 21:
		return "Constant"
	case 22:
		return "Struct"
	case 23:
		return "Event"
	case 24:
		return "Operator"
	case 25:
		return "TypeParameter"
	default:
		return fmt.Sprintf("Unknown(%d)", kind)
	}
}

func TestCompletionE2E(t *testing.T) {
	lspBinary := filepath.Join(t.TempDir(), "openchoreo-lsp")
	if runtime.GOOS == "windows" {
		lspBinary += ".exe"
	}
	build := exec.Command("go", "build", "-o", lspBinary, "github.com/openchoreo/openchoreo/cmd/openchoreo-lsp")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build LSP: %v\n%s", err, output)
	}
	cmd := exec.Command(lspBinary)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("start binary: %v", err)
	}
	defer func() {
		if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			t.Error(err)
		}
	}()

	writer := bufio.NewWriter(stdin)
	reader := bufio.NewReader(stdout)

	initParams := map[string]interface{}{
		"processId": os.Getpid(),
		"capabilities": map[string]interface{}{
			"textDocument": map[string]interface{}{
				"completion": map[string]interface{}{
					"completionItem": map[string]interface{}{
						"snippetSupport": true,
					},
				},
			},
		},
		"rootUri": "file:///tmp/test-project",
	}

	reqID := 1
	fmt.Println("=== Sending initialize request ===")
	if err := sendRequest(writer, reqID, "initialize", initParams); err != nil {
		t.Fatalf("send initialize: %v", err)
	}
	resp, err := readUntilResponse(reader, reqID)
	if err != nil {
		t.Fatalf("read initialize response: %v", err)
	}
	initResult, _ := json.MarshalIndent(resp.Result, "", "  ")
	fmt.Printf("Initialize response:\n%s\n\n", initResult)

	fmt.Println("=== Sending initialized notification ===")
	if err := sendNotification(writer, "initialized", map[string]interface{}{}); err != nil {
		t.Fatalf("send initialized: %v", err)
	}

	docURI := "file:///tmp/test-project/test.yaml"

	didOpenParams1 := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri":        docURI,
			"languageId": "yaml",
			"version":    1,
			"text":       "apiVersion: openchoreo.dev/v1alpha1\nkind: Environment\n",
		},
	}
	fmt.Println("=== Sending textDocument/didOpen (Environment, no metadata) ===")
	if err := sendNotification(writer, "textDocument/didOpen", didOpenParams1); err != nil {
		t.Fatalf("send didOpen 1: %v", err)
	}

	completionParams1 := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": docURI,
		},
		"position": map[string]interface{}{
			"line":      2,
			"character": 0,
		},
	}
	reqID = 2
	fmt.Println("=== Sending textDocument/completion at line 3, col 0 (after kind: Environment) ===")
	if err := sendRequest(writer, reqID, "textDocument/completion", completionParams1); err != nil {
		t.Fatalf("send completion 1: %v", err)
	}
	resp1, err := readUntilResponse(reader, reqID)
	if err != nil {
		t.Fatalf("read completion response 1: %v", err)
	}

	fmt.Println("\n========== COMPLETION RESULTS #1 (line 3, col 0, after kind: Environment) ==========")
	printCompletionResponse(resp1)

	didOpenParams2 := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri":        docURI,
			"languageId": "yaml",
			"version":    2,
			"text":       "apiVersion: openchoreo.dev/v1alpha1\nkind: Environment\nmetadata:\n",
		},
	}
	fmt.Println("\n=== Sending textDocument/didOpen (Environment, with metadata:) ===")
	if err := sendNotification(writer, "textDocument/didOpen", didOpenParams2); err != nil {
		t.Fatalf("send didOpen 2: %v", err)
	}

	completionParams2 := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": docURI,
		},
		"position": map[string]interface{}{
			"line":      3,
			"character": 0,
		},
	}
	reqID = 3
	fmt.Println("=== Sending textDocument/completion at line 4, col 0 (after metadata:) ===")
	if err := sendRequest(writer, reqID, "textDocument/completion", completionParams2); err != nil {
		t.Fatalf("send completion 2: %v", err)
	}
	resp2, err := readUntilResponse(reader, reqID)
	if err != nil {
		t.Fatalf("read completion response 2: %v", err)
	}

	fmt.Println("\n========== COMPLETION RESULTS #2 (line 4, col 0, after metadata:) ==========")
	printCompletionResponse(resp2)

	didOpenParams3 := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri":        docURI,
			"languageId": "yaml",
			"version":    3,
			"text":       "apiVersion: openchoreo.dev/v1alpha1\nkin",
		},
	}
	fmt.Println("\n=== Sending textDocument/didOpen (partial 'kin' - no colon, invalid YAML) ===")
	if err := sendNotification(writer, "textDocument/didOpen", didOpenParams3); err != nil {
		t.Fatalf("send didOpen 3: %v", err)
	}

	completionParams3 := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": docURI,
		},
		"position": map[string]interface{}{
			"line":      1,
			"character": 3,
		},
	}
	reqID = 4
	fmt.Println("=== Sending textDocument/completion at line 2, col 3 (after 'kin', no colon) ===")
	if err := sendRequest(writer, reqID, "textDocument/completion", completionParams3); err != nil {
		t.Fatalf("send completion 3: %v", err)
	}
	resp3, err := readUntilResponse(reader, reqID)
	if err != nil {
		t.Fatalf("read completion response 3: %v", err)
	}

	fmt.Println("\n========== COMPLETION RESULTS #3 (line 2, col 3, 'kin' no colon) ==========")
	printCompletionResponse(resp3)

	didOpenParams4 := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri":        docURI,
			"languageId": "yaml",
			"version":    4,
			"text":       "apiVersion: openchoreo.dev/v1alpha1\nkind: DataPlane\nmeta",
		},
	}
	fmt.Println("\n=== Sending textDocument/didOpen (DataPlane, partial 'meta' - invalid YAML) ===")
	if err := sendNotification(writer, "textDocument/didOpen", didOpenParams4); err != nil {
		t.Fatalf("send didOpen 4: %v", err)
	}

	completionParams4 := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": docURI,
		},
		"position": map[string]interface{}{
			"line":      2,
			"character": 4,
		},
	}
	reqID = 5
	fmt.Println("=== Sending textDocument/completion at line 3, col 4 (after 'meta', invalid YAML) ===")
	if err := sendRequest(writer, reqID, "textDocument/completion", completionParams4); err != nil {
		t.Fatalf("send completion 4: %v", err)
	}
	resp4, err := readUntilResponse(reader, reqID)
	if err != nil {
		t.Fatalf("read completion response 4: %v", err)
	}

	fmt.Println("\n========== COMPLETION RESULTS #4 (line 3, col 4, partial 'meta', invalid YAML) ==========")
	printCompletionResponse(resp4)

	shutdownID := 99
	fmt.Println("\n=== Sending shutdown request ===")
	if err := sendRequest(writer, shutdownID, "shutdown", nil); err != nil {
		t.Fatalf("send shutdown: %v", err)
	}
	_, _ = readUntilResponse(reader, shutdownID)

	fmt.Println("=== Sending exit notification ===")
	if err := sendNotification(writer, "exit", nil); err != nil {
		t.Fatalf("send exit: %v", err)
	}

	if err := cmd.Wait(); err != nil {
		fmt.Printf("Process exit: %v\n", err)
	}

	fmt.Println("\n=== Test completed ===")
}

func printCompletionResponse(response *jsonrpcMessage) {
	if response.Error != nil {
		fmt.Printf("ERROR: %v\n", response.Error)
	} else {
		resultBytes, _ := json.Marshal(response.Result)
		var compList completionList
		if err := json.Unmarshal(resultBytes, &compList); err != nil {
			var items []completionItem
			if err2 := json.Unmarshal(resultBytes, &items); err2 != nil {
				fmt.Printf("Raw result: %s\n", resultBytes)
			} else {
				compList.Items = items
			}
		}
		fmt.Printf("Total completion items: %d\n", len(compList.Items))
		for i, item := range compList.Items {
			fmt.Printf("  [%d] Label: %q | Kind: %s (%d) | InsertText: %q | Detail: %q\n",
				i+1, item.Label, completionKindString(item.Kind), item.Kind, item.InsertText, item.Detail)
		}
	}
}
