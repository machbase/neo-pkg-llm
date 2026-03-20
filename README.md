# neo-pkg-llm

Go로 작성된 Machbase Neo AI 어시스턴트 백엔드입니다. LLM 프로바이더(Claude, ChatGPT, Gemini, Ollama)를 [Machbase Neo](https://machbase.com) 시계열 데이터베이스에 연결하여 에이전틱 도구 호출 루프를 통해 자율적으로 작업을 수행합니다.

## 주요 기능

- **멀티 LLM 지원** - Claude, ChatGPT, Gemini, Ollama(로컬) 프로바이더/모델 실시간 전환 가능
- **에이전틱 루프** - LLM이 도구를 선택하고 실행한 뒤, 작업 완료까지 자율 반복
- **4가지 실행 모드**
  - `server` - SSE 스트리밍을 지원하는 HTTP API (`/api/chat/stream`)
  - `cli` - 대화형 터미널
  - `mcp` - MCP (Model Context Protocol) stdio 서버
  - `ws` - WebSocket 클라이언트 (Neo chatbot UI 연동)
- **Machbase Neo 도구** - SQL 쿼리, TQL 스크립트 실행/검증, 대시보드 CRUD, 파일 관리, 문서 검색
- **TQL 차트 템플릿** - 사전 정의된 분석 템플릿 (추세, 변동성, FFT, RMS, 엔벨로프 등) 자동 확장
- **가드 시스템** - LLM 파라미터명 자동 수정, 연속 실패 감지, 대시보드 조기 생성 방지

## 프로젝트 구조

```
neo-pkg-llm/
├── main.go              # 진입점 (server / cli / mcp / ws 모드)
├── ws.go                # WebSocket 클라이언트 (Neo 연동)
├── config.go            # 설정 로딩 (.env, config.json, 환경변수)
├── config.json          # 런타임 설정 파일
├── go.mod
├── agent/
│   ├── agent.go         # 에이전틱 루프, 도구 호출 수정, 가드 로직
│   └── templates.go     # TQL 분석 템플릿 로더/확장기
├── llm/
│   ├── types.go         # LLMProvider 인터페이스, Message, ToolCall 타입
│   ├── prompt.go        # 시스템 프롬프트
│   ├── claude.go        # Anthropic Claude 클라이언트
│   ├── chatgpt.go       # OpenAI ChatGPT 클라이언트
│   ├── gemini.go        # Google Gemini 클라이언트
│   └── ollama.go        # Ollama 로컬 LLM 클라이언트
├── machbase/
│   └── client.go        # Machbase Neo HTTP/API 클라이언트 (SQL, TQL, 파일)
├── mcp/
│   └── server.go        # MCP stdio JSON-RPC 서버
├── tools/
│   ├── registry.go      # 도구 레지스트리 및 실행
│   ├── sql.go           # SQL 도구 (list_tables, execute_sql_query 등)
│   ├── tql.go           # TQL 도구 (실행, 검증, 저장)
│   ├── dashboard.go     # 대시보드 CRUD 도구
│   ├── files.go         # 파일/폴더 관리 도구
│   ├── docs.go          # 문서 검색 도구
│   └── util.go          # 유틸리티 도구 (버전, 디버그)
└── neo/                 # Machbase Neo 문서 (md 파일)
    ├── api/
    ├── bridges/
    ├── dbms/
    ├── installation/
    ├── jsh/
    ├── operations/
    ├── security/
    ├── sql/
    ├── tql/
    └── utilities/
```

## 사전 요구사항

- Go 1.22 이상
- Machbase Neo 서버 실행 중

## 설정 예시

### config.json

```json
{
  "machbase": {
    "host": "127.0.0.1",
    "port": "5654",
    "user": "sys",
    "password": "manager"
  },
  "claude": {
    "api_key": "",
    "models": [{"name": "sonnet", "model_id": "claude-sonnet-4-20250514"}]
  },
  "chatgpt": {
    "api_key": "",
    "models": [{"name": "gpt-4o"}]
  },
  "gemini": {
    "api_key": "",
    "models": [{"name": "gemini-2.5-flash", "model_id": "gemini-2.5-flash-preview-04-17"}]
  },
  "ollama": {
    "base_url": "",
    "models": [{"name": "qwen3:8b"}]
  }
}
```

## 사용법

```bash

# 의존성 설치
go mod tidy

# 빌드
go build -o neo-pkg-llm.exe .

# HTTP 서버 모드 (기본값)
./neo-pkg-llm -mode server -port 8080

# CLI 모드
./neo-pkg-llm -mode cli
./neo-pkg-llm --mode cli --provider claude --model claude-haiku-4-5-20251001

# MCP stdio 서버 모드
./neo-pkg-llm -mode mcp

# WebSocket 클라이언트 모드 (Neo chatbot UI 연동 예시)
./neo-pkg-llm -mode ws -neo-ws-url "ws://127.0.0.1:5654/web/api/chat/data"
```

### CLI 플래그

| 플래그 | 기본값 | 설명 |
|---|---|---|
| `-mode` | `server` | 실행 모드: `server`, `cli`, `mcp`, `ws` |
| `-port` | `8080` | HTTP 서버 포트 (server 모드 전용) |
| `-neo-ws-url` | *(없음)* | Neo WebSocket URL (ws 모드 전용) |
| `-config` | `config.json` | 설정 파일 경로 |
| `-provider` | *(자동)* | LLM 프로바이더 오버라이드 |
| `-model` | *(자동)* | 모델 이름 또는 model_id 오버라이드 |

### API 엔드포인트 (Server 모드)

| 메서드 | 경로 | 설명 |
|---|---|---|
| `GET` | `/health` | 헬스 체크 |
| `GET` | `/settings` | 설정 페이지 |
| `GET` | `/api/settings` | 현재 설정 조회 |
| `POST` | `/api/settings` | 설정 저장 |
| `POST` | `/api/restart-llm` | LLM 클라이언트 재시작 |
| `POST` | `/api/chat` | 채팅 (비스트리밍) |
| `POST` | `/api/chat/stream` | 채팅 (SSE 스트리밍) |

### WebSocket 프로토콜 (ws 모드)

Neo와 neo-pkg-llm 간 양방향 통신에 사용됩니다.

**Neo → neo-pkg-llm:**

```json
{"type": "chat",  "session_id": "abc123", "query": "테이블 목록 보여줘"}
{"type": "stop",  "session_id": "abc123"}
```

**neo-pkg-llm → Neo:**

```json
{"type": "status",      "session_id": "abc123", "content": "에이전트 초기화 완료"}
{"type": "stream",      "session_id": "abc123", "content": "분석 결과를..."}
{"type": "tool_call",   "session_id": "abc123", "step": 1, "name": "list_tables"}
{"type": "tool_result", "session_id": "abc123", "status": "success", "content": "..."}
{"type": "final",       "session_id": "abc123", "content": "최종 응답"}
{"type": "stopped",     "session_id": "abc123", "content": "사용자에 의해 중단되었습니다."}
```

## 사용 가능한 도구

| 도구 | 설명 |
|---|---|
| `list_tables` | Machbase 테이블 목록 조회 |
| `list_table_tags` | 테이블 내 태그 목록 조회 |
| `execute_sql_query` | SQL 쿼리 실행 |
| `execute_tql_script` | TQL 스크립트 실행 |
| `validate_chart_tql` | TQL 차트 스크립트 검증 |
| `save_tql_file` | TQL 파일 저장 (실행 검증 포함) |
| `create_folder` | 폴더 생성 |
| `list_files` | 파일 목록 조회 |
| `delete_file` | 파일 삭제 |
| `create_dashboard` | 빈 대시보드 생성 |
| `create_dashboard_with_charts` | 차트 포함 대시보드 생성 |
| `add_chart_to_dashboard` | 대시보드에 차트 추가 |
| `remove_chart_from_dashboard` | 차트 제거 |
| `update_chart_in_dashboard` | 차트 수정 |
| `delete_dashboard` | 대시보드 삭제 |
| `update_dashboard_time_range` | 시간 범위 변경 |
| `preview_dashboard` | 대시보드 미리보기 |
| `list_available_documents` | Neo 문서 목록 조회 |
| `get_full_document_content` | 문서 내용 조회 |
| `get_document_sections` | 문서 섹션 조회 |
| `extract_code_blocks` | 문서에서 코드 블록 추출 |
| `get_version` | 버전 정보 조회 |
| `debug_mcp_status` | 연결 상태 확인 |
