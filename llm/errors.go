package llm

import (
	"fmt"
	"net/http"
)

// APIError represents a user-friendly LLM API error.
type APIError struct {
	Provider   string
	StatusCode int
	RawBody    string
}

func (e *APIError) Error() string {
	msg := friendlyMessage(e.Provider, e.StatusCode)
	return fmt.Sprintf("[%s] %s (HTTP %d)", e.Provider, msg, e.StatusCode)
}

// newAPIError creates a user-friendly API error from an HTTP response.
func newAPIError(provider string, statusCode int, rawBody string) *APIError {
	return &APIError{
		Provider:   provider,
		StatusCode: statusCode,
		RawBody:    rawBody,
	}
}

func friendlyMessage(provider string, code int) string {
	switch code {
	case http.StatusUnauthorized: // 401
		return "API key가 유효하지 않습니다. 설정을 확인해주세요."
	case http.StatusForbidden: // 403
		return "API 접근 권한이 없습니다."
	case http.StatusTooManyRequests: // 429
		return "API 사용량 한도를 초과했습니다. 잠시 후 다시 시도해주세요."
	case http.StatusInternalServerError: // 500
		return fmt.Sprintf("%s 서버에 일시적인 오류가 발생했습니다.", provider)
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout: // 502, 503, 504
		return fmt.Sprintf("%s 서버에 연결할 수 없습니다. 잠시 후 다시 시도해주세요.", provider)
	default:
		return fmt.Sprintf("API 요청이 실패했습니다. (상태 코드: %d)", code)
	}
}
