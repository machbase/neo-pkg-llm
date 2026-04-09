# /gen-page — 페이지 + 컴포넌트 생성

## 설명
라우트에 등록할 페이지와 해당 페이지의 섹션 컴포넌트를 레퍼런스 패턴에 맞게 생성한다.

## 사용법
```
/gen-page <PageName> [--type <dashboard|form>] [--sections <list>] [--route <path>]
```

- `PageName`: 페이지 이름 (예: AlertDashboard, AlertForm)
- `--type`: 페이지 유형 (기본값: `dashboard`)
  - `dashboard`: 읽기 전용 상세 뷰 (DashboardPage 패턴)
  - `form`: 생성/수정 폼 (JobFormPage 패턴)
- `--sections`: 섹션 컴포넌트 이름 (쉼표 구분)
- `--route`: 라우트 경로 (기본값: 자동 추론)

## 실행 절차

### Step 1: 레퍼런스 패턴 확인
페이지 유형에 따라 레퍼런스를 Read 도구로 읽는다:

**dashboard 유형:**
- `src/pages/DashboardPage.jsx`
- `src/components/dashboard/SourceConfigCard.jsx`
- `src/components/dashboard/TargetConfigCard.jsx`

**form 유형:**
- `src/pages/JobFormPage.jsx`
- `src/components/jobs/SourceSection.jsx`
- `src/components/jobs/ExecutionSection.jsx`
- `src/components/jobs/AdvancedSection.jsx`

### Step 2: 페이지 생성 (`src/pages/<PageName>Page.jsx`)

**dashboard 유형 구조:**
```jsx
import { useApp } from '../context/AppContext'
import Icon from '../components/common/Icon'
// 섹션 컴포넌트 import

export default function <PageName>Page({ items, onDelete }) {
  const { selectedItemId, setSelectedItemId } = useApp()

  return (
    <div className="page">
      <div className="page-body">
        <div className="page-body-inner">
          <div className="page-title-group">
            <h1 className="page-title">Page Title</h1>
            <p className="page-desc">Description text.</p>
          </div>
          {/* 섹션 카드들 (.card 클래스) */}
        </div>
      </div>
    </div>
  )
}
```

**form 유형 구조:**
```jsx
import { useState, useEffect } from 'react'
import { useNavigate, useParams } from 'react-router'
import { useApp } from '../context/AppContext'
// API import
// 섹션 컴포넌트 import

const DEFAULTS = { /* 폼 초기값 */ }

export default function <PageName>Page({ onRefresh }) {
  const { id } = useParams()
  const navigate = useNavigate()
  const { notify } = useApp()
  const isEdit = Boolean(id)
  const [form, setForm] = useState(DEFAULTS)

  // dot-notation 경로 기반 상태 업데이트
  const update = (path, value) => {
    setForm(prev => {
      const keys = path.split('.')
      const next = { ...prev }
      let cur = next
      for (let i = 0; i < keys.length - 1; i++) {
        cur[keys[i]] = { ...cur[keys[i]] }
        cur = cur[keys[i]]
      }
      cur[keys[keys.length - 1]] = value
      return next
    })
  }

  // handleSubmit: create or update API 호출

  return (
    <div className="page">
      <div className="page-body">
        <div className="page-body-inner">
          <div className="page-title-group">
            <h1 className="page-title">{isEdit ? 'Edit' : 'New'} ...</h1>
          </div>
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
            {/* 섹션 컴포넌트들 */}
          </div>
          <div className="flex justify-end gap-2 mt-6">
            <button className="btn btn-ghost" onClick={() => navigate('/')}>Cancel</button>
            <button className="btn btn-primary" onClick={handleSubmit}>
              {isEdit ? 'Update' : 'Create'}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
```

### Step 3: 섹션 컴포넌트 생성

각 섹션은 `src/components/<domain>/` 디렉토리에 생성.

**dashboard 섹션 패턴:**
```jsx
import Icon from '../common/Icon'

export default function <SectionName>({ data }) {
  return (
    <div className="card">
      <h3 className="card-title">
        <Icon name="<icon>" className="text-primary" />
        <span>Section Title</span>
      </h3>
      <div className="space-y-3">
        {/* 필드 표시: label + dash-field-box */}
      </div>
    </div>
  )
}
```

**form 섹션 패턴:**
```jsx
import Icon from '../common/Icon'

export default function <SectionName>({ form, update }) {
  return (
    <div className="form-card">
      <div className="form-card-header">
        <Icon name="<icon>" />
        <span>Section Title</span>
      </div>
      <div className="space-y-4">
        {/* 입력 필드: form-label + input */}
      </div>
    </div>
  )
}
```

### Step 4: 라우트 등록
`App.jsx`와 `IndexApp.jsx`에 라우트를 추가한다:
- dashboard: `<Route path="/<route>" element={<PageName>Page ... />} />`
- form: `<Route path="/<domain>/new" .../>` + `<Route path="/<domain>/:id/edit" .../>`

### Step 5: 디자인 검증
`design-system` 에이전트를 호출하여 Neo 디자인 시스템 준수 확인.

### Step 6: 결과 보고
생성된 파일 목록과 등록된 라우트를 사용자에게 보고.

## CSS 클래스 규칙 (엄격 준수)

| 용도 | 클래스 |
|------|--------|
| 페이지 뼈대 | `.page` + `.page-header` / `.page-body` / `.page-body-inner` |
| 풀하이트 페이지 | `.page-body-full` |
| 제목 그룹 | `.page-title-group` + `.page-title` + `.page-desc` |
| 탭 바 | `.tab-bar` + `.tab-item` |
| 카드 (읽기) | `.card` + `.card-title` / `.card-header` |
| 카드 설명 | `.card-desc` |
| 카드 (폼) | `.form-card` + `.form-card-header` |
| 폼 필드 래퍼 | `.form-field` |
| 폼 2열 그리드 | `.form-row` |
| 필드 값 표시 | `.dash-field-box` |
| 라벨 (읽기) | `.label` |
| 라벨 (폼) | `.form-label` |
| 연결 입력 | `.input-group` + `.input-group-addon` |
| 스위치 행 | `.switch-row` + `.switch-row-label` + `.switch-row-desc` |
| 버튼 | `.btn` + `.btn-primary` / `.btn-ghost` / `.btn-danger` / `.btn-success` |
| 배지 | `.badge` + `.badge-success` / `.badge-error` / `.badge-warning` / `.badge-primary` |
| 태그 | `.tag` + `.tag-match` / `.tag-error` / `.tag-live` |
| 테이블 | `.table` (th/td 자동 스타일) |
| 리스트 아이템 | `.list-item` + `.list-item-body` / `.list-item-title` |
| 빈 상태 | `.empty-state` |
| 에러 박스 | `.error-box` |
| 페이지네이션 | `.pagination` |
| 모달 | `.modal-overlay` + `.modal` + `.modal-sm/md/lg` |
| 드롭다운 | `.dropdown-menu` + `.dropdown-option` |
| 비디오 | `.video-container` |
| 아이콘 | `<Icon name="..." />` (Material Symbols) |
| 접이식 | `<details className="card">` + `<summary>` |
