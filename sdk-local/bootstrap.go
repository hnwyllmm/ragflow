// Provision disposable SDK tenants through real HTTP APIs, without provider keys.
package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

type object = map[string]any
type api struct{ base, auth string }

func (a *api) call(method, path string, body object) (object, http.Header, error) {
	b, _ := json.Marshal(body)
	req, err := http.NewRequest(method, a.base+"/api/v1"+path, bytes.NewReader(b))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if a.auth != "" {
		req.Header.Set("Authorization", a.auth)
	}
	res, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("%s %s: transport failed", method, path)
	}
	defer res.Body.Close()
	var data object
	err = json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&data)
	if err != nil || res.StatusCode != 200 || data["code"] != float64(0) {
		return nil, nil, fmt.Errorf("%s %s: HTTP %d API %v (response redacted)", method, path, res.StatusCode, data["code"])
	}
	return data, res.Header, nil
}
func tenant(base, password string) (*api, string, string, error) {
	nonce := make([]byte, 16)
	if _, e := rand.Read(nonce); e != nil {
		return nil, "", "", e
	}
	email := "seekdb-sdk-" + hex.EncodeToString(nonce) + "@example.invalid"
	a := &api{base: base}
	if _, _, e := a.call("POST", "/users", object{"email": email, "nickname": "SeekDB isolated SDK", "password": password}); e != nil {
		return nil, "", "", e
	}
	login, h, e := a.call("POST", "/auth/login", object{"email": email, "password": password})
	if e != nil {
		return nil, "", "", e
	}
	a.auth = h.Get("Authorization")
	profile, _ := login["data"].(map[string]any)
	id, _ := profile["id"].(string)
	if id == "" || a.auth == "" {
		return nil, "", "", fmt.Errorf("login missing tenant ID or authorization")
	}
	tok, _, e := a.call("POST", "/system/tokens", nil)
	if e != nil {
		return nil, "", "", e
	}
	d, _ := tok["data"].(map[string]any)
	key, _ := d["token"].(string)
	if key == "" {
		return nil, "", "", fmt.Errorf("missing API token")
	}
	return a, id, key, nil
}
func run() error {
	if len(os.Args) != 4 {
		return fmt.Errorf("usage: bootstrap base-url pinned-configs.py private-output.json")
	}
	base := strings.TrimRight(os.Args[1], "/")
	if base != "http://127.0.0.1:9380" {
		return fmt.Errorf("only disposable loopback CI stack is allowed")
	}
	source, e := os.ReadFile(os.Args[2])
	if e != nil {
		return e
	}
	m := regexp.MustCompile(`(?s)PASSWORD = """(.*?)"""`).FindSubmatch(source)
	if len(m) != 2 {
		return fmt.Errorf("pinned upstream password fixture not found")
	}
	owner, id, key, e := tenant(base, string(m[1]))
	if e != nil {
		return e
	}
	_, otherID, otherKey, e := tenant(base, string(m[1]))
	if e != nil {
		return e
	}
	_, _, e = owner.call("PATCH", "/models/default", object{"model_provider": "Builtin", "model_instance": "Local", "model_type": "embedding", "model_name": "BAAI/bge-small-en-v1.5"})
	if e != nil {
		return e
	}
	env := map[string]string{"RAGFLOW_SDK_ISOLATED": "1", "RAGFLOW_SDK_BASE_URL": base, "RAGFLOW_SDK_API_KEY": key, "RAGFLOW_SDK_TENANT_ID": id, "RAGFLOW_SDK_OTHER_API_KEY": otherKey, "RAGFLOW_SDK_OTHER_TENANT_ID": otherID, "RAGFLOW_SDK_DOC_ENGINE": "seekdb", "RAGFLOW_SDK_EMBEDDING_MODEL": "BAAI/bge-small-en-v1.5@Local@Builtin"}
	b, e := json.Marshal(env)
	if e != nil {
		return e
	}
	return os.WriteFile(os.Args[3], b, 0600)
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
