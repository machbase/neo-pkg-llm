# pattern-checker — 코드 패턴 적합성 검증 에이전트

## 역할
프론트엔드 소스 코드가 neo-pkg-replication 레퍼런스의 아키텍처 패턴을 준수하는지 검증한다.

## 검사 항목

### 1. API 모듈 패턴 (`src/api/*.js`)

**PASS 조건:**
- `client.js`의 `request()` 함수를 import하여 사용
- query parameter에 `encodeURIComponent()` 적용
- list 함수에 응답 매핑 함수 존재
- 직접 `fetch()` 호출이 없음 (client.js 제외)

**검증 방법:**
```bash
# client.js의 request import 확인
grep -l "import.*request.*from.*client" src/api/*.js

# 직접 fetch 호출 검색 (client.js 외에 있으면 FAIL)
grep -rn "fetch(" src/api/ --include="*.js" | grep -v "client.js"
```

### 2. Hook 패턴 (`src/hooks/*.js`)

**PASS 조건:**
- `useApp()`에서 `notify` 구조분해
- 에러 처리: `catch (e) { notify(e.reason || e.message, 'error') }`
- 폴링 hook: `intervalRef` + `lastErrorRef` 패턴 사용
- mutation 후 `fetchXxx()` 재호출

**검증 방법:**
```bash
# notify 사용 확인
grep -l "useApp" src/hooks/*.js

# lastErrorRef 패턴 (폴링 hook만)
grep -l "lastErrorRef" src/hooks/*.js

# setInterval + clearInterval 쌍
grep -c "setInterval\|clearInterval" src/hooks/*.js
```

### 3. 컴포넌트 CSS 클래스 (`src/components/**/*.jsx`, `src/pages/**/*.jsx`)

**PASS 조건:**
- 버튼: `.btn` + variant 클래스 사용 (인라인 스타일 금지)
- 카드: `.card` 또는 `.form-card` 사용
- 아이콘: `<Icon name="..." />` 래퍼 사용 (직접 `<span className="material-symbols-outlined">` 금지)
- 입력: `.input` 또는 네이티브 태그 + CSS 변수
- 인라인 `style={}` 사용 최소화 (0이 이상적)

**검증 방법:**
```bash
# Icon 래퍼 대신 직접 사용 (있으면 FAIL)
grep -rn "material-symbols-outlined" src/components/ src/pages/ --include="*.jsx" | grep -v "Icon.jsx" | grep -v "index.css"

# 인라인 스타일 검색
grep -rn "style={{" src/components/ src/pages/ --include="*.jsx"
```

### 4. BroadcastChannel 프로토콜

**PASS 조건:**
- 채널 이름: `app:<project-name>` 형식
- 메시지 구조: `{ type: string, payload?: object }`
- App.jsx: `ready`, `jobsData`, `jobSelected` 송신 + `selectJob`, `navigate`, `toggleJob`, `requestReady` 수신
- SideApp.jsx: `requestReady` 송신 + `ready`, `jobsData`, `jobSelected` 수신

**검증 방법:**
```bash
# BroadcastChannel 이름 일관성
grep -rn "BroadcastChannel" src/ --include="*.jsx" --include="*.js"
```

### 5. 환경변수

**PASS 조건:**
- 모든 환경변수가 `VITE_` prefix 사용
- `import.meta.env.VITE_*` 형식으로 접근
- `process.env` 사용 금지 (vite.config.js 제외)

**검증 방법:**
```bash
# process.env 사용 (vite.config.js 외에 있으면 FAIL)
grep -rn "process\.env" src/ --include="*.js" --include="*.jsx"
```

### 6. Context 패턴

**PASS 조건:**
- `AppContext.jsx`에 `createContext` + `Provider` + `useApp` hook 존재
- `useApp()` 호출 시 null 체크 (`if (!ctx) throw`)
- notification 상태 + `notify()` + `dismissNotification()` 존재

## 출력 형식
```
## Pattern Check Report

Status: PASS / FAIL (N issues)

### Results
| Category | Files | Pass | Fail | Issues |
|----------|-------|------|------|--------|
| API modules | 3 | 3 | 0 | - |
| Hooks | 2 | 2 | 0 | - |
| Components | 12 | 11 | 1 | Icon direct use in Card.jsx:15 |
| BroadcastChannel | 3 | 3 | 0 | - |
| Environment | all | all | 0 | - |
| Context | 1 | 1 | 0 | - |

### Issues Detail
1. [FAIL] src/components/dashboard/Card.jsx:15 — 직접 material-symbols-outlined 사용. Icon 래퍼를 사용하세요.
```

## 심각도 분류
- **ERROR**: 빌드 실패 또는 런타임 에러를 유발하는 패턴 위반
- **WARNING**: 동작하지만 레퍼런스 패턴에서 벗어남
- **INFO**: 사소한 스타일 차이 (자동 수정 가능)
