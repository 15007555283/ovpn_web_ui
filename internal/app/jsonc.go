package app

import (
	"encoding/json"
	"errors"
)

// 仅移除字符串之外的注释，保留换行和偏移；URL 中的 // 不受影响。
func decodeJSONC(b []byte, out any) error {
	clean := append([]byte(nil), b...)
	quoted, escaped := false, false
	for i := 0; i < len(clean); i++ {
		if quoted {
			if escaped {
				escaped = false
			} else if clean[i] == '\\' {
				escaped = true
			} else if clean[i] == '"' {
				quoted = false
			}
			continue
		}
		if clean[i] == '"' {
			quoted = true
			continue
		}
		if clean[i] != '/' || i+1 >= len(clean) {
			continue
		}
		if clean[i+1] == '/' {
			for i < len(clean) && clean[i] != '\n' {
				clean[i] = ' '
				i++
			}
		} else if clean[i+1] == '*' {
			clean[i], clean[i+1] = ' ', ' '
			i += 2
			for i+1 < len(clean) && !(clean[i] == '*' && clean[i+1] == '/') {
				if clean[i] != '\n' && clean[i] != '\r' {
					clean[i] = ' '
				}
				i++
			}
			if i+1 >= len(clean) {
				return errors.New("JSONC 块注释未结束")
			}
			clean[i], clean[i+1] = ' ', ' '
			i++
		}
	}
	return json.Unmarshal(clean, out)
}
