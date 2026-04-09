# /scaffold — 새 프론트엔드 프로젝트 초기화

## 설명
현재 neo-pkg-replication 레포의 프론트엔드 아키텍처를 기반으로, 새 레포에 동일한 기술 스택과 디렉토리 구조를 생성한다.

## 사용법
```
/scaffold <project-name> [--proxy <url>] [--api-base <path>]
```

- `project-name`: 프로젝트 이름 (package.json name, BroadcastChannel name 등에 사용)
- `--proxy`: 개발 서버 프록시 타겟 (기본값: `http://localhost:5654`)
- `--api-base`: API base path (기본값: `/public/<project-name>`)

## 실행 절차

### Step 1: 인자 파싱
사용자 입력에서 project-name, proxy, api-base를 추출한다. 미지정 시 기본값 적용.

### Step 2: 디렉토리 구조 생성
대상 경로에 다음 구조를 생성한다:

```
├── index.html
├── main.html
├── side.html
├── package.json
├── vite.config.js
├── styles/
│   └── index.css              ← Single Source of Truth (프로젝트 루트)
└── src/
    ├── main.jsx               ← import '../styles/index.css'
    ├── index-main.jsx
    ├── side-main.jsx
    ├── App.jsx
    ├── IndexApp.jsx
    ├── SideApp.jsx
    ├── api/
    │   └── client.js
    ├── context/
    │   └── AppContext.jsx
    ├── hooks/
    ├── pages/
    └── components/
        ├── common/
        │   ├── Icon.jsx
        │   ├── Toast.jsx
        │   ├── StatusBadge.jsx
        │   └── ConfirmDialog.jsx
        ├── layout/
        │   └── Sidebar.jsx
        ├── dashboard/
        ├── jobs/
        └── servers/
```

### Step 3: 파일별 생성 규칙

#### package.json
레퍼런스 package.json을 읽고 동일한 의존성으로 생성.
- `name`을 `<project-name>-web`으로 변경
- scripts의 build 명령은 동일 (멀티 엔트리 빌드)
- dependencies: react ^19, react-dom ^19, react-router ^7
- devDependencies: @tailwindcss/vite ^4, @vitejs/plugin-react ^4, tailwindcss ^4, vite ^6, vite-plugin-singlefile ^2

#### vite.config.js
레퍼런스 vite.config.js를 읽고 동일 구조로 생성.
- proxy 경로를 `--api-base` 값으로 변경
- proxy target을 `--proxy` 값으로 변경

#### HTML 엔트리 (3개)
레퍼런스의 index.html, main.html, side.html과 동일 구조.
- title을 `<project-name>` 기반으로 변경
- Google Fonts CDN 링크 (Inter + Material Symbols) 동일 유지

#### src/api/client.js
레퍼런스 src/api/client.js를 **그대로 복사**.
- `VITE_API_BASE` 기본값만 `--api-base`로 변경

#### src/context/AppContext.jsx
레퍼런스를 **그대로 복사**. (selectedJobId → selectedItemId로 네이밍만 범용화)

#### styles/index.css
레퍼런스의 Neo 디자인 시스템 CSS를 **전체 복사**. 이 파일은 수정 없이 그대로 사용한다.
프로젝트 루트의 `styles/index.css`가 Single Source of Truth이다. `src/styles/` 에는 두지 않는다.

#### Entry Points (main.jsx, index-main.jsx, side-main.jsx)
레퍼런스와 동일 구조. BroadcastChannel 이름을 `app:<project-name>`으로 변경.

#### App.jsx, IndexApp.jsx, SideApp.jsx
레퍼런스와 동일 구조로 생성. 라우트는 빈 상태로 `/`만 등록.
- BroadcastChannel 이름을 `app:<project-name>`으로 변경
- 도메인별 import/hook은 제거하고 빈 셸만 남김

#### Common Components (Icon, Toast, StatusBadge, ConfirmDialog)
레퍼런스를 **그대로 복사**.

#### Sidebar.jsx
레퍼런스 구조를 기반으로 빈 사이드바 생성. 아이템 목록은 props로 받되 비어있는 상태.

### Step 4: npm install 실행
```bash
npm install
```

### Step 5: 빌드 검증
`build-validator` 에이전트를 호출하여 3개 엔트리 빌드 성공 확인.

### Step 6: 결과 보고
생성된 파일 목록과 빌드 결과를 사용자에게 보고.

## 중요 규칙
- 레퍼런스 레포의 실제 파일을 Read 도구로 읽어서 복사한다. 기억에 의존하지 않는다.
- Neo 디자인 시스템 CSS (index.css)는 절대 수정하지 않는다.
- BroadcastChannel 프로토콜 (ready, requestReady, jobsData, jobSelected, selectJob, navigate, toggleJob)은 동일하게 유지한다.
- vite-plugin-singlefile 설정은 반드시 포함한다.
