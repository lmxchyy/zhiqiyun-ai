package providerexecution

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// Fingerprint hashes only immutable logical request identity. encoding/json
// emits map keys in lexical order, making this stable across key insertion order.
func Fingerprint(taskID, provider, model, capability string, params any) (string, error) {
	v := struct {
		TaskID     string `json:"task_id"`
		Provider   string `json:"provider"`
		Model      string `json:"model"`
		Capability string `json:"capability"`
		Params     any    `json:"params"`
	}{taskID, provider, model, capability, params}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}
