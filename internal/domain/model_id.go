package domain

import (
	"fmt"
)

// ValidateCLIModelID checks the literal model identifier accepted by CLI settings.
func ValidateCLIModelID(id string) error {
	const maxCLIModelIDLen = 128
	if id == "" {
		return fmt.Errorf("模型 ID 不能为空")
	}
	if len(id) > maxCLIModelIDLen {
		return fmt.Errorf("模型 ID 长度不能超过 %d 个字符", maxCLIModelIDLen)
	}
	for _, r := range id {
		if r <= ' ' || r == 0x7f || r == '"' || r == '\'' || r == '`' || r == '\\' {
			return fmt.Errorf("模型 ID %q 不能包含空格、引号、反斜杠或控制字符", id)
		}
	}
	return nil
}
