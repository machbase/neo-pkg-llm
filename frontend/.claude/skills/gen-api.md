# /gen-api — API 레이어 생성

## 설명
새 도메인의 API 모듈과 대응하는 데이터 Hook을 레퍼런스 패턴에 맞게 생성한다.

## 사용법
```
/gen-api <domain> [--endpoints <list>] [--polling <interval>] [--base-path <path>]
```

- `domain`: 도메인 이름 (예: alerts, dashboards, metrics)
- `--endpoints`: 쉼표 구분 엔드포인트 (기본값: `list,get,create,update,delete`)
- `--polling`: 폴링 주기 (예: `5s`, `none`. 기본값: `5s`)
- `--base-path`: API 경로 접두사 (예: `/cgi-bin/api/alerts`. 기본값: `/cgi-bin/api/<domain>`)

## 실행 절차

### Step 1: 레퍼런스 패턴 확인
반드시 아래 파일을 Read 도구로 읽어서 최신 패턴을 확인한다:
- `src/api/client.js` — request() 함수 시그니처
- `src/api/jobs.js` — API 모듈 패턴
- `src/hooks/useJobs.js` — Hook 패턴

### Step 2: API 모듈 생성 (`src/api/<domain>.js`)
레퍼런스 jobs.js 패턴을 따른다:

```javascript
import { request } from './client'

const BASE = '<base-path>'

// list 응답 매핑 함수
function mapListItem(item) {
  return { ...item, id: item.name, status: item.running ? 'running' : 'stopped' }
}

// CRUD + 커스텀 액션
export const list<Domain> = async () => { ... }
export const get<Domain> = async (id) => { ... }
export const create<Domain> = (data) => request('POST', BASE, data)
export const update<Domain> = (id, data) => request('PUT', `${BASE}?name=${encodeURIComponent(id)}`, data)
export const delete<Domain> = (id) => request('DELETE', `${BASE}?name=${encodeURIComponent(id)}`)
```

규칙:
- 반드시 `client.js`의 `request()`를 사용한다
- query parameter는 `encodeURIComponent()`로 인코딩
- list 응답의 매핑 함수를 별도로 분리한다

### Step 3: Hook 생성 (`src/hooks/use<Domain>.js`)
레퍼런스 useJobs.js 패턴을 따른다:

```javascript
import { useState, useEffect, useCallback, useRef } from 'react'
import * as <domain>Api from '../api/<domain>'
import { useApp } from '../context/AppContext'

export default function use<Domain>() {
  const [items, setItems] = useState([])
  const [loading, setLoading] = useState(true)
  const { notify } = useApp()
  const intervalRef = useRef(null)
  const lastErrorRef = useRef(null)

  const fetchItems = useCallback(async () => {
    try {
      const data = await <domain>Api.list<Domain>()
      setItems(data)
      lastErrorRef.current = null
    } catch (e) {
      const msg = e.reason || e.message
      if (lastErrorRef.current !== msg) {
        lastErrorRef.current = msg
        notify(msg, 'error')
      }
    } finally {
      setLoading(false)
    }
  }, [notify])

  useEffect(() => {
    fetchItems()
    if (<polling>) {
      intervalRef.current = setInterval(fetchItems, <interval_ms>)
      return () => clearInterval(intervalRef.current)
    }
  }, [fetchItems])

  // toggle, remove 등 mutation 메서드...

  return { items, loading, ..., refresh: fetchItems }
}
```

규칙:
- `useApp()`의 `notify()`를 에러/성공 알림에 사용
- 에러 중복 알림 방지를 위해 `lastErrorRef` 패턴 적용
- 폴링이 `none`이면 `setInterval` 생략, mount 시 1회만 fetch
- mutation 메서드는 `try/catch + notify + fetchItems()`

### Step 4: 패턴 검증
`pattern-checker` 에이전트를 호출하여 생성된 코드의 패턴 준수를 확인한다.

### Step 5: 결과 보고
생성된 파일과 export 목록을 사용자에게 보고.

## 에러 처리 패턴 (엄격 준수)

| 메서드 유형 | catch 처리 | throw 전파 |
|-------------|-----------|-----------|
| fetch (목록 조회) | notify만 | X (삼킴) |
| mutation (CRUD) | notify | 선택적 throw (caller 흐름 제어용) |
| toggle (시작/정지) | notify | X (삼킴) |
