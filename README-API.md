# neo-pkg-llm API Reference

Machbase Neo AI Assistant 백엔드.
LLM 프로바이더(Claude, ChatGPT, Gemini, Ollama)와 Machbase Neo를 연결하는 Agentic Tool-Calling 서버.

---

## 아키텍처

```
                        ┌─────────────────────────────────────┐
                        │         Master Server (:8884)        │
                        │                                     │
                        │  /health          전체 헬스체크      │
                        │  /settings        설정 페이지        │
                        │  /api/instances   인스턴스 목록       │
                        │  /api/configs     Config CRUD        │
                        │                                     │
                        │  /{name}/...  ──┐                   │
                        └─────────────────┼───────────────────┘
                                          │
                    ┌─────────────────────┼─────────────────────┐
                    │                     │                     │
              ┌─────▼──────┐       ┌──────▼─────┐       ┌──────▼─────┐
              │ Instance A │       │ Instance B │       │ Instance C │
              │ (alice)    │       │ (bob)      │       │ (carol)    │
              ├────────────┤       ├────────────┤       ├────────────┤
              │ machbase   │       │ machbase   │       │ machbase   │
              │ registry   │       │ registry   │       │ registry   │
              │ LLM client │       │ LLM client │       │ LLM client │
              │ wsServer   │       │ wsServer   │       │ wsServer   │
              └────────────┘       └────────────┘       └────────────┘
```

- `configs/` 디렉토리의 JSON 파일 1개 = Instance 1개 (고루틴)
- 서버 시작 시 `configs/*.json`을 전부 로드하여 Instance 자동 생성
- Config CRUD API로 런타임에 Instance 추가/수정/삭제 가능

---

## 실행 모드

```bash
# HTTP 서버 (멀티 인스턴스)
./neo-pkg-llm -mode server -port 8884

# 대화형 CLI
./neo-pkg-llm -mode cli -config config.json

# MCP 서버 (stdio JSON-RPC)
./neo-pkg-llm -mode mcp

# WebSocket 클라이언트 (Neo 연결)
./neo-pkg-llm -mode ws --neo-ws-url "ws://host:port/web/api/llm/chat"
```

### CLI 플래그

| 플래그 | 기본값 | 설명 |
|--------|--------|------|
| `-mode` | `server` | 실행 모드: `server`, `cli`, `mcp`, `ws` |
| `-port` | (config) | HTTP 서버 포트 (server 모드) |
| `-config` | `config.json` | Config 파일 경로 |
| `-provider` | (auto) | LLM 프로바이더 override |
| `-model` | (auto) | 모델 이름 override |
| `--neo-ws-url` | | Neo WebSocket URL (ws 모드 필수) |

---

## Config 구조

### config.json (마스터 설정)

```json
{
  "server": { "port": "8884" },
  "machbase": {
    "host": "127.0.0.1",
    "port": "5654",
    "user": "sys",
    "work_dir": ""
  },
  "claude": {
    "api_key": "",
    "models": [
      { "name": "sonnet", "model_id": "claude-sonnet-4-20250514" },
      { "name": "haiku", "model_id": "claude-haiku-4-5-20251001" }
    ]
  },
  "chatgpt": {
    "api_key": "",
    "models": [
      { "name": "gpt-4o" },
      { "name": "gpt-4o-mini" }
    ]
  },
  "gemini": {
    "api_key": "",
    "models": [
      { "name": "gemini-2.5-flash", "model_id": "gemini-2.5-flash-preview-04-17" }
    ]
  },
  "ollama": {
    "base_url": "",
    "models": [{ "name": "qwen3:8b" }],
    "temperature": 0.7,
    "num_predict": 2048,
    "num_ctx": 4096,
    "num_gpu": 0
  }
}
```

### configs/{user}.json (유저별 설정)

마스터 config과 동일한 구조. `machbase.user` 값이 파일명이 됨.

### 환경변수 Override

| 환경변수 | 대상 |
|----------|------|
| `MACHBASE_HOST` | machbase.host |
| `MACHBASE_PORT` | machbase.port |
| `MACHBASE_USER` | machbase.user |
| `MACHBASE_WORK_DIR` | machbase.work_dir |
| `LLM_PROVIDER` | provider (claude/chatgpt/gemini/ollama) |
| `LLM_MODEL` | model name |
| `ANTHROPIC_API_KEY` | claude.api_key |
| `OPENAI_API_KEY` | chatgpt.api_key |
| `GEMINI_API_KEY` | gemini.api_key |
| `OLLAMA_BASE_URL` | ollama.base_url |

---

## API Endpoints

### Master API (인스턴스 공통)

#### `GET /health`

서버 전체 헬스체크.

```json
// Response
{ "status": "ok" }
```

#### `GET /api/instances`

현재 실행 중인 모든 Instance 목록.

```json
// Response
{
  "success": true,
  "reason": "success",
  "elapse": "42µs",
  "data": {
    "instances": [
      {
        "name": "alice",
        "provider": "claude",
        "model": "claude-sonnet-4-20250514",
        "machbase_url": "http://192.168.1.100:5654"
      },
      {
        "name": "bob",
        "provider": "ollama",
        "model": "qwen3:8b",
        "machbase_url": "http://127.0.0.1:5654"
      }
    ]
  }
}
```

---

### Config CRUD API

모든 응답은 공통 포맷:

```json
{
  "success": true|false,
  "reason": "success" | "에러 메시지",
  "elapse": "83.2µs",
  "data": { ... }
}
```

#### `POST /api/configs`

Config 저장 + Instance 시작.

```bash
curl -X POST http://localhost:8884/api/configs \
  -H "Content-Type: application/json" \
  -d '{
    "server": { "port": "8884" },
    "machbase": {
      "host": "192.168.1.100",
      "port": "5654",
      "user": "alice",
      "work_dir": "/data/alice"
    },
    "claude": {
      "api_key": "sk-ant-...",
      "models": [{ "name": "sonnet", "model_id": "claude-sonnet-4-20250514" }]
    }
  }'
```

```json
// Response
{ "success": true, "reason": "success", "data": { "name": "alice" } }
```

- `configs/alice.json`으로 저장
- 같은 이름의 Instance가 있으면 stop → 재시작
- LLM 초기화 실패 시: config은 저장되지만 Instance는 미시작 (reason에 에러 포함)

**필수 필드**: `machbase.host`, `machbase.port`, `machbase.user`, `machbase.work_dir`

#### `GET /api/configs`

저장된 Config 목록 (실행 상태 포함).

```json
{
  "data": {
    "configs": [
      { "name": "alice", "running": true },
      { "name": "bob", "running": false }
    ]
  }
}
```

#### `GET /api/configs/{name}`

특정 Config 조회.

```json
{
  "data": {
    "config": { "server": {...}, "machbase": {...}, ... },
    "running": true
  }
}
```

#### `PUT /api/configs/{name}`

Config 수정 + Instance 재시작.

- `machbase.user`가 변경되면 파일명도 변경 (rename)
- 기존 Instance stop → 새 config으로 start

#### `DELETE /api/configs/{name}`

Config 삭제 + Instance 종료.

- config 파일 삭제
- 해당 Instance의 모든 WebSocket 세션 cancel & close
- Instance map에서 제거

---

### Per-Instance API (`/{name}/...`)

`{name}`은 `machbase.user` 값 (= config 파일명).

#### `GET /{name}/health`

Instance 헬스체크.

```json
{
  "status": "ok",
  "instance": "alice",
  "provider": "claude",
  "model": "claude-sonnet-4-20250514"
}
```

#### `GET /{name}/api/settings`

Instance의 현재 Config 반환.

#### `POST /{name}/api/restart-llm`

Instance의 LLM 클라이언트 재시작.

```json
// Response
{ "status": "restarted", "provider": "claude", "model": "claude-sonnet-4-20250514" }
```

#### `POST /{name}/api/chat`

비스트리밍 채팅.

```bash
curl -X POST http://localhost:8884/alice/api/chat \
  -H "Content-Type: application/json" \
  -d '{ "query": "테이블 목록 보여줘" }'
```

```json
// Response
{ "result": "현재 등록된 테이블은 다음과 같습니다:\n..." }
```

#### `POST /{name}/api/chat/stream`

SSE 스트리밍 채팅.

```bash
curl -N -X POST http://localhost:8884/alice/api/chat/stream \
  -H "Content-Type: application/json" \
  -d '{ "query": "테이블 목록 보여줘" }'
```

```
data: {"type":"status","content":"Calling LLM..."}
data: {"type":"stream","content":"현재 "}
data: {"type":"stream","content":"등록된 "}
data: {"type":"tool_call","step":1,"name":"list_tables","args":{}}
data: {"type":"tool_result","status":"success","content":"..."}
data: {"type":"final","content":"완료"}
```

#### `WS /{name}/ws`

WebSocket 연결 (Chat UI용).

---

### WebSocket 프로토콜

#### Client → Server

```json
// 채팅 요청
{
  "type": "chat",
  "user_id": "alice",
  "session_id": "sess-uuid",
  "provider": "claude",
  "model": "sonnet",
  "query": "온도 태그 목록 보여줘"
}

// 중지
{ "type": "stop", "session_id": "sess-uuid" }

// 모델 목록 요청
{ "type": "get_models", "user_id": "alice" }
```

#### Server → Client

```json
// 모델 목록 응답
{
  "type": "models",
  "providers": [
    {
      "provider": "claude",
      "models": [
        { "name": "sonnet", "model_id": "claude-sonnet-4-20250514" }
      ]
    }
  ]
}

// 채팅 응답 (Legacy Neo Chat UI 포맷)
{
  "type": "msg",
  "session": "sess-uuid",
  "message": {
    "ver": "1.0",
    "id": 0,
    "type": "stream_block_delta",
    "body": {
      "ofStreamBlockDelta": {
        "contentType": "text",
        "text": "응답 내용..."
      }
    }
  }
}
```

#### 세션 관리

- TTL: **30분** (비활성 시 자동 만료)
- Provider/Model 변경 시 Agent 리셋
- 유저 소유권 검증 (다른 유저의 세션 접근 차단)
- Instance 삭제 시 해당 세션 전부 cancel + close

---

## 프로젝트 구조

```
neo-pkg-llm/
├── main.go              # 진입점, 4개 모드 분기
├── config.go            # Config 로딩, 환경변수 override
├── manager.go           # 멀티 인스턴스 관리, URL 라우팅
├── instance.go          # 인스턴스별 HTTP 핸들러
├── ws_server.go         # WebSocket 서버 (server 모드)
├── ws.go                # WebSocket 클라이언트 (ws 모드)
├── handler_configs.go   # API 응답 포맷
├── configs_api_test.go  # 테스트
│
├── agent/
│   ├── agent.go         # Agentic Loop (LLM ↔ Tool 반복)
│   └── templates.go     # TQL 분석 템플릿
│
├── llm/
│   ├── types.go         # LLMProvider 인터페이스
│   ├── prompt.go        # 시스템 프롬프트
│   ├── claude.go        # Claude (Anthropic)
│   ├── chatgpt.go       # ChatGPT (OpenAI)
│   ├── gemini.go        # Gemini (Google)
│   └── ollama.go        # Ollama (로컬)
│
├── machbase/
│   └── client.go        # Machbase Neo HTTP 클라이언트
│
├── mcp/
│   └── server.go        # MCP stdio 서버
│
├── tools/
│   ├── registry.go      # 도구 레지스트리
│   ├── sql.go           # SQL 도구 (list_tables, execute_sql_query, ...)
│   ├── tql.go           # TQL 도구 (execute_tql_script, validate_chart_tql, ...)
│   ├── dashboard.go     # 대시보드 도구 (create/update/delete dashboard, ...)
│   ├── files.go         # 파일 도구 (create_folder, list_files, ...)
│   ├── docs.go          # 문서 도구 (list_available_documents, ...)
│   └── util.go          # 유틸 도구 (get_version, debug_mcp_status, ...)
│
└── configs/             # 유저별 Config (런타임 생성)
    ├── alice.json
    └── bob.json
```

---

## 등록된 도구 목록

### SQL
| 도구 | 설명 |
|------|------|
| `list_tables` | 테이블 목록 조회 |
| `list_table_tags` | 태그 목록 조회 |
| `execute_sql_query` | SQL 쿼리 실행 |

### TQL
| 도구 | 설명 |
|------|------|
| `execute_tql_script` | TQL 스크립트 실행 |
| `validate_chart_tql` | 차트 TQL 검증 |
| `save_tql_file` | TQL 파일 저장 |

### Dashboard
| 도구 | 설명 |
|------|------|
| `create_dashboard` | 빈 대시보드 생성 |
| `create_dashboard_with_charts` | 차트 포함 대시보드 생성 |
| `add_chart_to_dashboard` | 차트 추가 |
| `remove_chart_from_dashboard` | 차트 제거 |
| `update_chart_in_dashboard` | 차트 수정 |
| `delete_dashboard` | 대시보드 삭제 |
| `update_dashboard_time_range` | 시간 범위 변경 |
| `preview_dashboard` | 대시보드 미리보기 |
| `get_dashboard` | 대시보드 조회 |

### Files
| 도구 | 설명 |
|------|------|
| `create_folder` | 폴더 생성 |
| `list_files` | 파일 목록 |
| `delete_file` | 파일 삭제 |

### Docs
| 도구 | 설명 |
|------|------|
| `list_available_documents` | 문서 목록 |
| `get_full_document_content` | 문서 전체 조회 |
| `get_document_sections` | 문서 섹션 조회 |
| `extract_code_blocks` | 코드 블록 추출 |

### Utility
| 도구 | 설명 |
|------|------|
| `get_version` | 버전 정보 |
| `debug_mcp_status` | MCP 상태 디버그 |
| `update_connection` | 연결 정보 변경 |
