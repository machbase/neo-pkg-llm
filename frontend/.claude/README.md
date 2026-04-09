# Claude Code — neo-pkg-replication 프론트엔드 복제 자동화

이 프로젝트의 프론트엔드 아키텍처를 다른 레포에 동일하게 재현하기 위한 Skills + Agents 시스템.

## 개요

```
사용자 워크플로우:

  /scaffold ──→ 프로젝트 초기화 (빌드 가능한 빈 프로젝트)
  /gen-api  ──→ API 레이어 생성 (api 모듈 + hook)
  /gen-page ──→ 페이지 + 컴포넌트 생성 (라우트 자동 등록)
  /verify   ──→ 빌드 검증 + 패턴 적합성 체크

  ┌──────────────────────────────────┐
  │       Sub-Agents (자동 호출)       │
  │  build-validator    빌드 검증      │
  │  pattern-checker    패턴 준수 검사  │
  │  design-system      디자인 일관성   │
  │  api-layer-generator API 코드 생성 │
  └──────────────────────────────────┘
```

## 기술 스택 (레퍼런스)

| 항목 | 버전 |
|------|------|
| React | 19 |
| React Router | 7 (HashRouter) |
| Tailwind CSS | 4 |
| Vite | 6 |
| vite-plugin-singlefile | 2.3 |

## 아키텍처 특징

- **멀티 엔트리**: `index.html`(통합), `main.html`(메인), `side.html`(사이드바) 3개 독립 빌드
- **BroadcastChannel**: 창 간 상태 동기화 (`app:<project-name>`)
- **Single-file 번들**: 각 엔트리를 단일 HTML로 번들링 (Neo 프레임워크 임베딩용)
- **Neo 디자인 시스템**: 다크 테마, Material Symbols, 커스텀 CSS 클래스
- **CGI 기반 백엔드**: `/cgi-bin/api/` 경로로 통신

---

## Skills

### `/scaffold` — 프로젝트 초기화

새 레포에 동일한 기술 스택 + 디렉토리 구조를 생성한다.

```
/scaffold <project-name> [--proxy <url>] [--api-base <path>]
```

**생성물:**
- `package.json`, `vite.config.js`, HTML 엔트리 3개
- `src/` 전체 디렉토리 구조 (api, context, hooks, pages, components, styles)
- 공통 컴포넌트 (Icon, Toast, StatusBadge, ConfirmDialog)
- Neo 디자인 시스템 CSS 전체

**보장:** `npm install && npm run build` 즉시 성공

---

### `/gen-api` — API 레이어 생성

새 도메인의 API 모듈 + 데이터 Hook을 생성한다.

```
/gen-api <domain> [--endpoints <list>] [--polling <interval>] [--base-path <path>]
```

**생성물:**
- `src/api/<domain>.js` — CRUD + 커스텀 액션
- `src/hooks/use<Domain>.js` — 폴링 + notify 패턴

**패턴 규칙:**
- `client.js`의 `request()` 사용 (직접 fetch 금지)
- `encodeURIComponent()` 적용
- Hook: `lastErrorRef` 에러 중복 방지, `intervalRef` 폴링
- 에러: `notify(e.reason || e.message, 'error')`

---

### `/gen-page` — 페이지 + 컴포넌트 생성

라우트 페이지와 섹션 컴포넌트를 생성한다.

```
/gen-page <PageName> [--type <dashboard|form>] [--sections <list>] [--route <path>]
```

**dashboard 유형:** 읽기 전용 상세 뷰 (`.card` + `.card-title`)
**form 유형:** 생성/수정 폼 (`.form-card` + `update(path, value)` 상태관리)

**생성물:**
- `src/pages/<PageName>Page.jsx`
- `src/components/<domain>/<Section>.jsx` (N개)
- `App.jsx` / `IndexApp.jsx` 라우트 자동 등록

---

### `/verify` — 빌드 검증 + 패턴 체크

두 에이전트를 **병렬 실행**하여 프로젝트 품질을 검증한다.

```
/verify [--build-only] [--pattern-only]
```

**출력:**
```
Build:   ✅ index.html (150KB) | main.html (120KB) | side.html (45KB)
Pattern: ✅ API 3/3 | Hooks 2/2 | Components 12/12
```

---

## Agents

### `build-validator`

| 항목 | 내용 |
|------|------|
| 역할 | `npm run build` 실행 + 산출물 검증 |
| 검사 | 빌드 성공, dist/ 파일 존재, single-file 여부, 파일 크기 |
| 호출 | `/verify`, `/scaffold` 완료 후 |

### `pattern-checker`

| 항목 | 내용 |
|------|------|
| 역할 | 소스 코드 패턴 적합성 검증 |
| 검사 | API 모듈, Hook, CSS 클래스, BroadcastChannel, 환경변수, Context |
| 호출 | `/verify`, `/gen-api`, `/gen-page` 완료 후 |

### `design-system`

| 항목 | 내용 |
|------|------|
| 역할 | Neo 디자인 시스템 일관성 검증 |
| 검사 | 하드코딩 색상, Icon 래퍼, 인라인 스타일, 컴포넌트 클래스, 폰트 |
| 호출 | `/gen-page` 완료 후 |

### `api-layer-generator`

| 항목 | 내용 |
|------|------|
| 역할 | `/gen-api`의 실제 코드 생성 |
| 동작 | 레퍼런스 읽기 → API 모듈 생성 → Hook 생성 → 패턴 검증 |
| 호출 | `/gen-api` 내부 |

### `migrator`

| 항목 | 내용 |
|------|------|
| 역할 | 기존 프론트엔드를 Neo 아키텍처로 마이그레이션 |
| 원칙 | 기존 기능 무손실 보존 + 점진적 패턴 전환 |
| 절차 | 인프라 업그레이드 → 디자인 시스템 → 공통 컴포넌트 → API 레이어 → 상태관리 → 멀티엔트리 |
| 검증 | 각 Phase 완료 후 빌드 검증 + 기능 회귀 체크 |

---

## 워크플로우 예시

새 프로젝트 "neo-pkg-monitoring" 생성:

```bash
# 1. 프로젝트 부트스트랩
/scaffold neo-pkg-monitoring --proxy http://localhost:5654

# 2. API 레이어
/gen-api alerts --endpoints "list,get,create,delete,acknowledge" --polling 5s
/gen-api dashboards --endpoints "list,get,update" --polling none

# 3. 페이지
/gen-page AlertDashboard --type dashboard --sections "AlertHeader,AlertList,AlertDetail"
/gen-page AlertForm --type form --sections "ConditionSection,NotifySection,AdvancedSection"

# 4. 전체 검증
/verify
```

---

## 품질 보장

```
모든 생성 스킬 ──→ 자동 pattern-checker 호출
                    │
                    ├─ PASS → 완료
                    └─ FAIL → 위반 목록 + 수정 제안 → 재검증
```

---

## 파일 구조

```
.claude/
├── README.md              ← 이 파일
├── skills/
│   ├── scaffold.md        ← /scaffold 스킬 정의
│   ├── gen-api.md         ← /gen-api 스킬 정의
│   ├── gen-page.md        ← /gen-page 스킬 정의
│   └── verify.md          ← /verify 스킬 정의
└── agents/
    ├── build-validator.md     ← 빌드 검증 에이전트
    ├── pattern-checker.md     ← 패턴 적합성 에이전트
    ├── design-system.md       ← 디자인 시스템 에이전트
    ├── api-layer-generator.md ← API 생성 에이전트
    └── migrator.md            ← 기존 FE 마이그레이션 에이전트
```

---

## 핵심 레퍼런스 파일

스킬/에이전트가 참조하는 레퍼런스 소스:

| 파일 | 역할 |
|------|------|
| `src/api/client.js` | fetch 래퍼 + ApiError (모든 API 모듈의 기반) |
| `src/api/jobs.js` | API 모듈 패턴 레퍼런스 |
| `src/hooks/useJobs.js` | Hook 패턴 레퍼런스 (폴링 + notify) |
| `src/context/AppContext.jsx` | Context 패턴 (selectedId + notifications) |
| `src/styles/index.css` | Neo 디자인 시스템 전체 (수정 금지, 그대로 복사) |
| `src/App.jsx` | BroadcastChannel 수신 패턴 |
| `src/SideApp.jsx` | BroadcastChannel 송신 패턴 |
| `vite.config.js` | 멀티 엔트리 + singlefile 빌드 설정 |
