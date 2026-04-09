# migrator — 기존 프론트엔드 Neo 아키텍처 마이그레이션 에이전트

## 역할
기존 프론트엔드의 **모든 기능을 보존**하면서 neo-pkg-replication 아키텍처 패턴으로 전환한다.

## 대상
`neo-pkg-blackbox/frontend` — React 18 + TypeScript + Vite 5, 탭 기반 설정 UI

## 현행 → 목표 비교

| 항목 | 현행 (blackbox) | 목표 (Neo 패턴) |
|------|----------------|----------------|
| React | 18.3 | 19 |
| Vite | 5.4 | 6 |
| CSS | 커스텀 App.css (자체 다크 테마) | Tailwind 4 + Neo 디자인 시스템 (index.css) |
| 라우팅 | 없음 (탭 전환) | React Router 7 (HashRouter) |
| 엔트리 | 단일 (index.html) | 멀티 (index.html, main.html, side.html) |
| API | configApi.ts (직접 fetch) | client.js 래퍼 + 도메인 모듈 |
| 상태관리 | 로컬 useState | AppContext + 커스텀 Hook |
| 알림 | saveStatusMessage 로컬 | Toast 컴포넌트 + notify() |
| 창간 통신 | 없음 | BroadcastChannel |
| 언어 | TypeScript | TypeScript 유지 (레퍼런스는 JS지만 TS 보존) |

## 핵심 원칙

1. **기능 무손실**: 마이그레이션 전후 모든 기능이 동일하게 동작해야 한다
2. **TypeScript 유지**: blackbox가 TS이므로 Neo 패턴을 TS로 변환 적용
3. **점진적 전환**: 한 레이어씩 바꾸고, 각 단계마다 빌드 검증
4. **데이터 보존**: configMapper, 타입 정의, API 엔드포인트 로직은 그대로 유지

---

## 마이그레이션 절차

### Phase 1: 인프라 업그레이드

#### Step 1.1 — 의존성 업그레이드
```bash
npm install react@^19 react-dom@^19 react-router@^7
npm install -D tailwindcss@^4 @tailwindcss/vite@^4 vite@^6 @vitejs/plugin-react@^4 vite-plugin-singlefile@^2
npm install -D @types/react@^19 @types/react-dom@^19
```

#### Step 1.2 — vite.config.ts 멀티 엔트리 전환
레퍼런스: `neo-pkg-replication/frontend/vite.config.js`를 Read하여 동일 구조로 변환.

```typescript
// 핵심 변경사항:
// 1. @tailwindcss/vite 플러그인 추가
// 2. rollupOptions.input에 멀티 엔트리 설정
// 3. proxy 설정: /api → http://[hostname]:8000
// 4. VITE_ENTRY 환경변수 기반 빌드 분기
```

**기존 보존**: `server.host: true`, `server.port: 5173` 유지

#### Step 1.3 — HTML 엔트리 3개 생성
- `index.html` — 기존 파일 수정 (Google Fonts CDN 추가: Inter + Material Symbols)
- `main.html` — 새로 생성 (메인 콘텐츠 전용)
- `side.html` — 새로 생성 (사이드바 전용)

**기존 보존**: `<div id="root">` 구조 유지

#### Step 1.4 — 빌드 검증
```bash
npm run build
```
3개 엔트리 모두 빌드 성공 확인. 실패 시 의존성 충돌 해결.

---

### Phase 2: 디자인 시스템 전환

#### Step 2.1 — Neo CSS 도입
레퍼런스: `neo-pkg-replication/frontend/styles/index.css`를 Read하여 `styles/index.css`(프로젝트 루트)로 복사.
`styles/index.css`가 Single Source of Truth — `src/styles/`에는 두지 않는다.

#### Step 2.2 — 엔트리에 Tailwind import 추가
```typescript
import '../styles/index.css';
```
기존 `App.css` import는 일단 유지 (병행 기간).

#### Step 2.3 — 컴포넌트 CSS 클래스 매핑
기존 blackbox CSS 클래스 → Neo 클래스 대응표:

| 기존 (App.css) | Neo (index.css) | 비고 |
|---------------|----------------|------|
| `.settings-shell` | CSS Grid 직접 구성 | 멀티엔트리 레이아웃으로 대체 |
| `.sidebar-panel` | `.side` + `.side-header` + `.side-body` | |
| `.panel-card` | `.card` | |
| `.field-row` | `.form-card` 내부 flex | |
| `.btn-primary` | `.btn.btn-primary` | |
| `.btn-ghost` | `.btn.btn-ghost` | |
| `.toggle-*` | `.switch` + `.switch-thumb` | |
| `input[type="text"]` | 네이티브 + `.input` | |
| `select` | 네이티브 + CSS 변수 | |
| CSS 변수 (`--bg`, `--panel`, `--card`) | Neo 토큰 (`--color-surface`, `--color-surface-alt`) | |

#### Step 2.4 — 컴포넌트별 클래스 전환
각 컴포넌트 파일을 열고 기존 클래스를 Neo 클래스로 교체:

**순서:**
1. `Sidebar.tsx` → `.side`, `.side-header`, `.side-body`, `.side-item`
2. `TopBar.tsx` → Neo 버튼/헤더 클래스
3. `GeneralTab.tsx` → `.card`, `.form-label`, `.input`, `.switch`
4. `FFmpegTab.tsx` → `.card`, `.btn`, `.input`
5. `LogTab.tsx` → `.card`, `.form-label`, `.input`, `.switch`
6. `App.tsx` → 레이아웃 구조 변경

**기존 보존**: 모든 onChange 핸들러, 데이터 바인딩, 조건부 렌더링 로직 유지

#### Step 2.5 — App.css 제거
모든 컴포넌트가 Neo 클래스로 전환된 후 App.css 삭제. 빌드 검증.

---

### Phase 3: 공통 컴포넌트 도입

#### Step 3.1 — 공통 컴포넌트 복사 (TS 변환)
레퍼런스에서 Read하여 TypeScript로 변환:

| 레퍼런스 (JS) | 생성 (TS) | 변경사항 |
|-------------|----------|---------|
| `components/common/Icon.jsx` | `components/common/Icon.tsx` | Props 인터페이스 추가 |
| `components/common/Toast.jsx` | `components/common/Toast.tsx` | Notification 타입 적용 |
| `components/common/ConfirmDialog.jsx` | `components/common/ConfirmDialog.tsx` | Props 인터페이스 추가 |
| `components/common/StatusBadge.jsx` | `components/common/StatusBadge.tsx` | Props 인터페이스 추가 |

#### Step 3.2 — 기존 컴포넌트에 Icon 래퍼 적용
Material Symbols 직접 사용이 있다면 `<Icon name="..." />` 래퍼로 교체.

---

### Phase 4: API 레이어 전환

#### Step 4.1 — client.ts 생성
레퍼런스 `src/api/client.js`를 Read하여 TypeScript 버전 생성:

```typescript
// src/api/client.ts
export class ApiError extends Error {
  status: number;
  reason: string;
  constructor(status: number, reason: string) { ... }
}

export async function request<T>(method: string, path: string, body?: unknown): Promise<T> { ... }
```

**기존 보존**: API base URL 로직 유지
- 현행: `${window.location.protocol}//${window.location.hostname}:8000`
- 전환: `VITE_API_BASE` 환경변수 + 기존 URL을 기본값으로

#### Step 4.2 — configApi.ts → api/config.ts 리팩터링
기존 `services/configApi.ts`를 `api/config.ts`로 이동하고 `request()` 래퍼 사용:

```typescript
// 기존
const res = await fetch(`${apiBaseUrl()}/api/config`);
// 전환
const data = await request<ApiConfigData>('GET', '/api/config');
```

**기존 보존**:
- `ApiEnvelope<T>` 응답 구조 처리 (Neo의 `{ ok, data }` 패턴과 다름 — blackbox는 `{ success, reason, data }`)
- `client.ts`의 응답 파싱을 blackbox 형식에 맞게 조정
- `configMapper.ts` 로직 전체 유지

#### Step 4.3 — 타입 정의 이동
```
types/settings.ts   → 그대로 유지
types/configApi.ts  → 그대로 유지 (api/config.ts에서 import)
services/configMapper.ts → 그대로 유지 (api/config.ts에서 import)
```

---

### Phase 5: 상태관리 전환

#### Step 5.1 — AppContext.tsx 생성
레퍼런스 `src/context/AppContext.jsx`를 Read하여 TypeScript로 변환:

```typescript
// src/context/AppContext.tsx
interface AppContextValue {
  selectedTab: SettingsTab;           // blackbox 전용: 현재 탭
  setSelectedTab: (tab: SettingsTab) => void;
  notifications: Notification[];
  notify: (message: string, type: 'success' | 'error' | 'info') => void;
  dismissNotification: (id: number) => void;
}
```

**기존 보존**: `saveState`, `saveStatusMessage`는 notify()로 자연 전환

#### Step 5.2 — useConfig 커스텀 Hook 생성
기존 App.tsx의 설정 로직을 Hook으로 추출:

```typescript
// src/hooks/useConfig.ts
export function useConfig() {
  // 기존 App.tsx에서 추출:
  // - draft, shadow 상태
  // - loadConfig() 로직
  // - handleSave() 로직 → notify() 사용으로 전환
  // - handleGeneralChange, handleFFmpegChange, handleLogChange
  return { draft, loading, isDirty, save, updateGeneral, updateFFmpeg, updateLog };
}
```

**기존 보존**: configMapper 변환 로직, AbortController 취소 처리, 에러 핸들링 전부 유지

#### Step 5.3 — App.tsx에서 Hook 사용으로 전환
```typescript
// 기존: App.tsx 내부에 모든 상태 + 로직
// 전환: useConfig() + useApp()으로 분리
function App() {
  const { notify } = useApp();
  const { draft, loading, isDirty, save, updateGeneral, updateFFmpeg, updateLog } = useConfig();
  // ... 렌더링만 담당
}
```

---

### Phase 6: 멀티 엔트리 + BroadcastChannel

#### Step 6.1 — 엔트리 포인트 분리
레퍼런스의 `main.jsx`, `index-main.jsx`, `side-main.jsx` 패턴을 TS로 적용:

```
src/main.tsx       → IndexApp 렌더 (index.html용, 사이드바 포함)
src/main-main.tsx  → App 렌더 (main.html용, 콘텐츠만)
src/side-main.tsx  → SideApp 렌더 (side.html용, 사이드바만)
```

#### Step 6.2 — 라우팅 도입
기존 탭 → React Router 라우트 전환:

| 기존 탭 | 라우트 | 비고 |
|---------|--------|------|
| `general` | `/` 또는 `/general` | 기본 라우트 |
| `ffmpeg` | `/ffmpeg` | |
| `log` | `/log` | |

```typescript
// src/App.tsx
<HashRouter>
  <Routes>
    <Route path="/" element={<GeneralTab ... />} />
    <Route path="/ffmpeg" element={<FFmpegTab ... />} />
    <Route path="/log" element={<LogTab ... />} />
  </Routes>
</HashRouter>
```

**기존 보존**: 탭 전환 UX 동일 (사이드바 클릭 → 라우트 이동)

#### Step 6.3 — BroadcastChannel 통합
레퍼런스 패턴 적용 (채널명: `app:neo-blackbox`):

**App.tsx (수신):**
- `selectTab` → 탭/라우트 전환
- `navigate` → 라우트 이동
- `requestReady` → 현재 상태 전송

**SideApp.tsx (송신):**
- 탭 클릭 → `selectTab` 메시지 전송
- 저장 버튼 → `save` 메시지 전송

**기존 보존**: 단일 엔트리(index.html)에서도 모든 기능이 동작해야 함

#### Step 6.4 — IndexApp.tsx 생성
사이드바 + 메인 콘텐츠를 통합하는 레이아웃:

```typescript
// src/IndexApp.tsx
function IndexApp() {
  return (
    <AppProvider>
      <div className="flex h-screen">
        <Sidebar />
        <main className="flex-1 overflow-auto">
          <App />
        </main>
      </div>
      <Toast />
    </AppProvider>
  );
}
```

---

### Phase 7: 최종 검증

#### Step 7.1 — 빌드 검증
`build-validator` 에이전트 호출. 3개 엔트리 모두 빌드 성공 확인.

#### Step 7.2 — 패턴 검증
`pattern-checker` 에이전트 호출. 모든 카테고리 PASS 확인.

#### Step 7.3 — 디자인 시스템 검증
`design-system` 에이전트 호출. 하드코딩 색상/인라인 스타일 0 확인.

#### Step 7.4 — 기능 회귀 체크리스트

| 기능 | 검증 방법 |
|------|----------|
| 설정 로드 (GET /api/config) | 페이지 로드 시 폼 필드에 값 채워짐 |
| 설정 저장 (POST /api/config) | Save 클릭 → 성공 토스트 |
| General 탭 모든 필드 | 서버, Machbase, MediaMTX, FFmpeg 설정 편집 가능 |
| FFmpeg 탭 probe args | 추가/삭제/수정 동작 |
| Log 탭 모든 필드 | 레벨, 포맷, 출력, 파일 설정 편집 가능 |
| 토글 스위치 | Machbase useToken, Log compress 동작 |
| 비밀번호 표시/숨기기 | API Token 필드 동작 |
| 변경 감지 (isDirty) | 수정 시 Save 버튼 활성화 |
| 에러 처리 | API 실패 시 에러 토스트 표시 |
| 탭 전환 | 사이드바 클릭으로 탭 이동 |
| 멀티 엔트리 | index/main/side.html 각각 독립 동작 |
| BroadcastChannel | main↔side 창 간 탭 동기화 |

---

## 실행 시 참조할 레퍼런스 파일

### Neo 패턴 레퍼런스 (Read 대상)
| 파일 | 용도 |
|------|------|
| `neo-pkg-replication/frontend/vite.config.js` | 멀티 엔트리 빌드 설정 |
| `neo-pkg-replication/frontend/src/api/client.js` | API 래퍼 패턴 |
| `neo-pkg-replication/frontend/src/context/AppContext.jsx` | Context 패턴 |
| `neo-pkg-replication/frontend/src/hooks/useJobs.js` | Hook 패턴 |
| `neo-pkg-replication/frontend/src/App.jsx` | BroadcastChannel + 라우팅 |
| `neo-pkg-replication/frontend/src/SideApp.jsx` | BroadcastChannel 송신 |
| `neo-pkg-replication/frontend/styles/index.css` | Neo 디자인 시스템 전체 (프로젝트 루트) |
| `neo-pkg-replication/frontend/src/components/common/Icon.jsx` | Icon 래퍼 |
| `neo-pkg-replication/frontend/src/components/common/Toast.jsx` | Toast 컴포넌트 |

### Blackbox 보존 대상 (수정 시 주의)
| 파일 | 보존 내용 |
|------|----------|
| `src/services/configMapper.ts` | fromApiToDraft, toPostPayload 로직 전체 |
| `src/types/settings.ts` | SettingsDraft, GeneralSettings 등 타입 전체 |
| `src/types/configApi.ts` | ApiConfigData, ApiEnvelope 타입 전체 |
| `src/data/mockSettings.ts` | 폴백 기본값 |
| `src/tabs/GeneralTab.tsx` | 필드 구성 + onChange 로직 (CSS만 전환) |
| `src/tabs/FFmpegTab.tsx` | probe args 추가/삭제 로직 (CSS만 전환) |
| `src/tabs/LogTab.tsx` | 필드 구성 + onChange 로직 (CSS만 전환) |

---

## 중요 주의사항

1. **API 응답 형식 차이**: blackbox는 `{ success, reason, data }`, replication은 `{ ok, reason, data }`. client.ts에서 blackbox 형식을 지원하도록 조정할 것.
2. **TypeScript 유지**: 레퍼런스가 JS여도 blackbox의 TS 기반을 유지한다. 타입 안전성을 해치지 않는다.
3. **configMapper 보존**: 이 파일은 API↔UI 데이터 변환의 핵심. 리팩터링 대상이 아니다.
4. **CSS 변수 충돌**: 기존 App.css의 `--bg`, `--panel` 등과 Neo의 `--color-surface` 등이 충돌하지 않도록 Phase 2에서 순차 전환.
5. **단일 엔트리 호환**: index.html 하나로도 모든 기능이 동작해야 한다. 멀티 엔트리는 추가 옵션.
