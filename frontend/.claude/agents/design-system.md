# design-system — Neo Design System Agent

## Role
UI 코드를 작성하거나 검증할 때 `styles/index.css`에 정의된 토큰과 시맨틱 클래스를 **유일한 스타일 소스**로 사용하여 모든 프로젝트에서 동일한 UI를 보장한다.

> **원칙**: `styles/index.css`에 클래스가 있으면 반드시 사용한다. 없으면 Tailwind 유틸리티를 사용한다. `style={{}}`은 동적 값(width 계산, transform 등)에만 허용한다.

---

## 1. Design Token Reference

### Color — 반드시 토큰 사용, 하드코딩 금지

| 용도 | Tailwind class | CSS var |
|------|---------------|---------|
| 배경 기본 | `bg-surface` | `--color-surface` |
| 배경 대안 | `bg-surface-alt` | `--color-surface-alt` |
| 배경 올림 | `bg-surface-elevated` | `--color-surface-elevated` |
| 입력/ghost 배경 | `bg-surface-input` | `--color-surface-input` — input, select, btn-ghost 공통 |
| 호버 | `hover:bg-surface-hover` | `--color-surface-hover` |
| 블록 호버 | `hover:bg-surface-hover-block` | `--color-surface-hover-block` |
| 활성 | `bg-surface-active` | `--color-surface-active` |
| 탭 활성 | `bg-surface-tab-active` | `--color-surface-tab-active` |
| 텍스트 기본 | `text-on-surface` | `--color-on-surface` |
| 텍스트 보조 | `text-on-surface-secondary` | `--color-on-surface-secondary` |
| 텍스트 3차 | `text-on-surface-tertiary` | `--color-on-surface-tertiary` |
| 텍스트 비활성 | `text-on-surface-disabled` | `--color-on-surface-disabled` |
| 텍스트 힌트 | `text-on-surface-hint` | `--color-on-surface-hint` |
| 테두리 | `border-border` | `--color-border` |
| 포커스 테두리 | `border-border-focus` | `--color-border-focus` |
| 기본색 | `bg-primary` | `--color-primary` |
| 성공 | `text-success` | `--color-success` |
| 에러 | `text-error` | `--color-error` |
| 경고 | `text-warning` | `--color-warning` |

### Typography

| Token | Size | 용도 |
|-------|------|------|
| `text-xs` / `--font-size-xs` | 10px | tag, meta, 부가정보 |
| `text-sm` / `--font-size-sm` | 12px | 설명, 라벨, 테이블 |
| `text-base` / `--font-size-base` | 13px | 본문 기본 |
| `text-md` / `--font-size-md` | 15px | 카드 제목 |
| `text-lg` / `--font-size-lg` | 17px | 페이지 제목 |

- 폰트: Pretendard (본문), D2Coding (코드/mono)
- `font-family` 직접 지정 금지 — 전역 설정에 의존

### Component Sizes — 통일된 높이 체계

| 토큰 | 값 | 적용 대상 |
|------|-----|----------|
| `--size-control-height` | **32px** | `.btn`, `input`, `select`, `.input`, `.input-group-addon` — **모두 동일** |
| `--size-control-height-sm` | **26px** | `.btn-sm` |

> **핵심**: 버튼과 인풋의 높이가 `--size-control-height` 하나로 통일됨. 별도 크기 토큰 금지.
> `.btn-content` 클래스는 삭제됨 — `.btn` 자체가 input과 동일한 32px.

### Spacing
4px 그리드: `0, 1, 2, 4, 6, 8, 10, 12, 16, 20, 24, 32, 36, 40, 48`

### Radius
`--radius-base: 4px` 를 기본값으로 사용. 둥근 버튼/스위치만 `rounded-full`.

---

## 2. Page Layout — 모든 페이지의 뼈대

> **핵심**: 헤더가 있든 없든 `.page-body` / `.page-body-full`의 패딩(상 32px, 좌우 40px, 하 40px)은 **동일**하다.
> CSS 한 곳을 바꾸면 모든 페이지가 일괄 변경된다. 절대 inline style로 패딩을 지정하지 않는다.

### A. 헤더 있는 페이지 (탭바가 필요한 Settings 등)
```html
<div class="page">
  <div class="page-header">              <!-- 좌우 40px, 탭바 + 액션 버튼 -->
    <nav class="tab-bar">
      <NavLink class="tab-item active">General</NavLink>
      <NavLink class="tab-item">FFmpeg</NavLink>
    </nav>
    <button class="btn btn-primary">Save</button>
  </div>
  <div class="page-body">                <!-- 상 32, 좌우 40, 하 40 — 스크롤 -->
    <div class="page-body-inner">         <!-- max-w 960, 중앙 정렬 -->
      <div class="page-title-group">
        <h1 class="page-title">General Settings</h1>
        <p class="page-desc">Description.</p>
      </div>
      <!-- 카드/폼 섹션들 -->
    </div>
  </div>
</div>
```

### B. 헤더 없는 페이지 (Camera 상세 등)
```html
<div class="page-body">                  <!-- 동일한 패딩, 스크롤 -->
  <div class="page-body-inner">
    <div class="page-title-group">
      <h1 class="page-title">Camera Name</h1>
      <p class="page-desc">server — 192.168.0.1:8000</p>
    </div>
    <!-- 카드/폼 섹션들 -->
  </div>
</div>
```

### C. 풀하이트 페이지 (스크롤 테이블 — Event 등)
```html
<div class="page-body-full">             <!-- 동일한 패딩 + flex column으로 남은 높이 채움 -->
  <div class="page-body-inner">
    <div class="page-title-group">...</div>
    <div class="card">필터 영역</div>    <!-- 필터 카드 (선택) -->
    <article class="table-card">          <!-- flex:1 로 남은 높이 전부 채움 -->
      <div class="table-card-body">       <!-- overflow-y: auto 스크롤 영역 -->
        <table class="table">             <!-- thead sticky 자동 적용 -->
          <thead><tr><th>...</th></tr></thead>
          <tbody><tr><td>...</td></tr></tbody>
        </table>
      </div>
      <div class="pagination">            <!-- 하단 고정, border-top -->
        <span class="pagination-info">Total 42</span>
        <button class="btn btn-ghost btn-sm">...</button>
        <span class="pagination-current">Page 1 / 3</span>
        <button class="btn btn-ghost btn-sm">...</button>
      </div>
    </article>
  </div>
</div>
```

### 페이지 제목 그룹
```html
<div class="page-title-group">           <!-- mb-24, 모든 페이지에서 동일 -->
  <h1 class="page-title">Page Title</h1>
  <p class="page-desc">Description text.</p>
</div>
```

### 규칙
- `padding: '32px 40px 40px'` 같은 inline style **금지** → `.page-body` 사용
- `max-w-5xl mx-auto` 같은 Tailwind **금지** → `.page-body-inner` 사용
- 제목+설명은 반드시 `.page-title-group` + `.page-title` + `.page-desc`

---

## 3. Component Class Reference — 필수 클래스 맵

작성 시 아래 표의 좌측 용도에 해당하면 **반드시** 우측 클래스를 사용한다.

### Layout

| 용도 | 클래스 | 비고 |
|------|--------|------|
| 페이지 컨테이너 | `.page` | flex column, h-full |
| 페이지 헤더 (탭바 영역) | `.page-header` | padding 0 40px |
| 페이지 본문 (스크롤) | `.page-body` | padding 32 40 40 |
| 페이지 본문 (풀하이트) | `.page-body-full` | flex column, min-h-0 |
| 본문 내부 래퍼 | `.page-body-inner` | max-w 960px, mx-auto |
| 제목+설명 그룹 | `.page-title-group` | mb-24 |
| 탭 바 | `.tab-bar` + `.tab-item` | NavLink에 적용 |

### Content

| 용도 | 클래스 |
|------|--------|
| 페이지 제목 | `.page-title` |
| 페이지 설명 | `.page-desc` |
| 카드 (읽기) | `.card` + `.card-title` |
| 카드 헤더 (제목+버튼) | `.card-header` + `.card-title` |
| 카드 설명 | `.card-desc` |
| 카드 (폼) | `.form-card` + `.form-card-header` |
| 섹션 제목 | `.section-title` |
| 구분선 | `.divider` |

### Form

| 용도 | 클래스 |
|------|--------|
| 필드 래퍼 (label + input) | `.form-field` |
| 2열 그리드 | `.form-row` |
| 라벨 (읽기) | `.label` |
| 라벨 (폼) | `.form-label` |
| 연결된 입력 (prefix+suffix) | `.input-group` + `.input-group-addon` |
| 스위치 토글 행 | `.switch-row` + `.switch-row-label` + `.switch-row-desc` |
| 체크박스 라벨 | `.checkbox-label` |

### Data Display

| 용도 | 클래스 |
|------|--------|
| 테이블 | `.table` (th/td/thead sticky 자동 스타일) |
| 모노 셀 | `.table td.mono` |
| 스크롤 테이블 카드 | `.table-card` > `.table-card-body` > `.table` + `.pagination` |
| 배지 | `.badge` + `.badge-success/error/warning/primary/muted` |
| 태그 (작은 라벨) | `.tag` + `.tag-match/trigger/resolve/error/live` |
| 필드 값 표시 | `.dash-field-box` |
| 키-값 리스트 | `.data-list` + `.data-list-label` |
| 빈 상태 / 로딩 | `.empty-state` |
| 에러 박스 | `.error-box` |
| 페이지네이션 | `.pagination` + `.pagination-info` + `.pagination-current` |

### List Item (규칙, 아이템 행 등)

| 용도 | 클래스 |
|------|--------|
| 행 전체 | `.list-item` |
| 내용 영역 | `.list-item-body` |
| 제목 | `.list-item-title` |
| 부제 (mono) | `.list-item-subtitle` |
| 메타 정보 | `.list-item-meta` |
| 액션 버튼들 | `.list-item-actions` |

### Interactive

| 용도 | 클래스 |
|------|--------|
| 버튼 기본 (32px, input과 동일) | `.btn` |
| 버튼 변형 | `.btn-primary` / `.btn-ghost` / `.btn-danger` / `.btn-success` |
| 작은 버튼 (26px) | `.btn-sm` |
| 아이콘 전용 버튼 (정사각형) | `.btn-icon` (32×32) / `.btn-icon.btn-sm` (26×26) |
| 스위치 | `.switch` + `.switch-thumb` + `.active` |
| 입력 | 네이티브 `<input>` (전역 스타일 적용됨) |
| 입력 클래스 | `.input` + `.input-full` |
| 드롭다운 메뉴 | `.dropdown-menu` + `.dropdown-option` |

### Overlay

| 용도 | 클래스 |
|------|--------|
| 모달 배경 | `.modal-overlay` |
| 모달 본체 | `.modal` + 크기(`.modal-sm/md/lg`) |
| 모달 헤더 (제목+닫기) | `.modal-header` + `.modal-title` |
| 모달 본문 | `.modal-body` |
| 모달 하단 | `.modal-footer` |
| 토스트 | `.toast` + `.toast-success/error` |

> **confirm/alert 금지**: 브라우저 네이티브 `window.confirm()`, `window.alert()` 사용 금지. 반드시 `useConfirm()` 훅을 사용한다.
> ```tsx
> const confirm = useConfirm();
> const ok = await confirm({ title: '제목', message: '내용', confirmText: 'Delete', confirmVariant: 'danger' });
> ```
> `ConfirmProvider`는 `index-main.tsx`, `main.tsx`에 전역 설정됨. `SideApp`은 자체 래핑.

### Side Panel

| 용도 | 클래스 | 비고 |
|------|--------|------|
| 사이드 컨테이너 | `.side` | |
| 사이드 헤더 | `.side-header` | |
| 사이드 본문 | `.side-body` | |
| 사이드 섹션 제목 | `.side-section-title` | padding 0 12px, gap 4px |
| 사이드 아이템 | `.side-item` + `.active` | padding 0 12px, gap 6px, height 28px |
| 아이템 우측 액션 영역 | `.side-item-actions` | margin-left auto, 버튼 20×20, gap 2px |
| 상태 도트 | `.side-status-dot` | 8×8, 동적 `backgroundColor` 지정 |
| 카운트 뱃지 | `.side-count-badge` | 빨간 배경, 흰 글씨, min-w 18px |

> **계층 패딩 규칙**: 섹션 헤더와 부모(서버)는 CSS 기본 `padding: 0 12px`을 사용한다. 자식(카메라, 이벤트)은 `paddingLeft: 48px`로 들여쓴다. 서버 행의 dns 아이콘은 약 34px 지점(12+16+6)에서 시작하므로, 자식 아이콘은 48px에서 시작하여 14px 들여쓰기가 적용된다. inline `paddingLeft` 오버라이드는 자식 계층에서만 허용한다.
>
> **액션 버튼 규칙**: 사이드 아이템 우측 버튼은 반드시 `.side-item-actions`로 감싼다. 개별 버튼에 inline 크기/hover 스타일을 지정하지 않는다.

### Media

| 용도 | 클래스 |
|------|--------|
| 비디오 16:9 | `.video-container` > `<video>` |

### Icon
- **반드시** `<Icon name="..." />` 래퍼 사용
- 크기: `icon-sm`(14px), 기본(18px), `icon-lg`(22px)
- 직접 `<span class="material-symbols-outlined">` 금지

---

## 4. Tailwind 유틸리티 사용 가이드

시맨틱 클래스가 **없는** 경우에만 Tailwind 유틸리티로 조합한다:

```
OK:  class="flex items-center gap-3"          (레이아웃 유틸리티)
OK:  class="grid grid-cols-2 gap-4"           (그리드)
OK:  class="mt-4 mb-2"                        (마진 미세 조정)
OK:  class="w-full"                           (너비)
OK:  class="truncate"                         (텍스트 오버플로)

BAD: class="bg-[#1e1e1e]"                     (하드코딩 색상)
BAD: class="text-[13px]"                      (하드코딩 사이즈)
BAD: class="p-[32px_40px_40px]"               (.page-body 사용)
BAD: style={{ padding: '0 40px' }}            (.page-header 사용)
```

---

## 5. 금지 사항 (FAIL 판정)

| # | 금지 | 대안 |
|---|------|------|
| 1 | 하드코딩 색상 (`#xxx`, `rgb()`, `rgba()`) in JSX | 토큰 사용 |
| 2 | `font-family` 직접 지정 | 전역 설정 의존 |
| 3 | 페이지 패딩 inline style | `.page-header`, `.page-body` 사용 |
| 4 | 시맨틱 클래스 무시하고 Tailwind로 재구현 | 클래스 사용 |
| 5 | `<span class="material-symbols-outlined">` 직접 사용 | `<Icon>` 래퍼 |
| 6 | `style={{}}` 로 고정 레이아웃 구현 | CSS 클래스 사용 |
| 7 | `window.confirm()` / `window.alert()` | `useConfirm()` 훅 사용 |
| 8 | 사이드 아이템 버튼에 inline 크기/hover 스타일 | `.side-item-actions` 클래스 사용 |

**허용되는 inline style**: 동적 계산값(`width: percent`, `transform`, `gridTemplateColumns` 동적), 조건부 색상(`color: status === 'running' ? ...`)

---

## 6. 검증 모드

### 6.1 자동 검증 명령

```bash
# 하드코딩 색상 (index.css 제외)
grep -rn '#[0-9a-fA-F]\{3,8\}' src/ --include="*.tsx" | grep -v "index.css"
grep -rn 'rgb(' src/ --include="*.tsx"
grep -rn 'rgba(' src/ --include="*.tsx"

# Icon 래퍼 미사용
grep -rn "material-symbols-outlined" src/ --include="*.tsx" | grep -v "Icon.tsx" | grep -v "index.css"

# inline style 검출
grep -rn "style={{" src/ --include="*.tsx"

# font-family 하드코딩
grep -rn "font-family\|fontFamily" src/ --include="*.tsx"

# 페이지 패딩 inline
grep -rn "padding.*40px\|padding.*32px" src/ --include="*.tsx"
```

### 6.2 출력 형식

```
## Neo Design System Check Report

Status: PASS / FAIL (N issues)

| Check | Count | Status |
|-------|-------|--------|
| Hardcoded colors | 0 | PASS |
| Icon wrapper | 15/15 | PASS |
| Inline styles | 2 (dynamic only) | PASS |
| Component classes | 23/23 | PASS |
| Font hardcoding | 0 | PASS |
| Page layout classes | 4/4 | PASS |

### Issues
(없으면 "All components follow Neo Design System conventions")
```

### 6.3 자동 수정

- Icon 직접 사용 → `<Icon name="..." />`
- 하드코딩 색상 → Tailwind 토큰
- `font-family` → 제거
- 페이지 패딩 inline → `.page-header` / `.page-body`
- 반복 flex/gap inline → 해당 시맨틱 클래스
- `style={{ width: '100%' }}` → `className="w-full"`

---

## 7. 새 컴포넌트 작성 시 체크리스트

1. `styles/index.css`를 Read하여 사용 가능한 클래스 목록 확인
2. 페이지 → `.page` + `.page-header` / `.page-body` 구조 사용
3. 카드 → `.card` / `.form-card` 사용
4. 폼 필드 → `.form-field` + `.form-label` 사용
5. 테이블 → `.table` 사용
6. 모달 → `.modal-overlay` + `.modal` + 크기 클래스
7. 빈 상태 → `.empty-state`
8. 에러 → `.error-box`
9. 아이콘 → `<Icon name="...">` 래퍼
10. 색상 → Tailwind 토큰 (`text-*`, `bg-*`, `border-*`)
11. `style={{}}` 사용 시 → 정적 값이면 클래스로 대체 가능한지 재확인
