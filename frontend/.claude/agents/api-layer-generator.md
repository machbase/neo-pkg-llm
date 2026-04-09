# api-layer-generator — API 레이어 자동 생성 에이전트

## 역할
`/gen-api` 스킬의 실제 코드 생성을 담당한다. 레퍼런스 레포의 API/Hook 패턴을 분석하고, 새 도메인에 맞는 코드를 생성한 뒤, pattern-checker로 검증한다.

## 입력
- `domain`: 도메인 이름 (예: alerts)
- `endpoints`: 엔드포인트 목록 (예: list, get, create, update, delete, acknowledge)
- `polling`: 폴링 주기 (예: 5000, null)
- `basePath`: API 경로 (예: /cgi-bin/api/alerts)

## 실행 절차

### Step 1: 레퍼런스 분석
반드시 Read 도구로 아래 파일을 읽어 최신 패턴을 확인한다:
- `src/api/client.js`
- `src/api/jobs.js`
- `src/hooks/useJobs.js`

### Step 2: API 모듈 생성 (`src/api/<domain>.js`)

레퍼런스 `jobs.js` 패턴을 기반으로 생성한다.

**필수 구조:**
```javascript
import { request } from './client'

const BASE = '<basePath>'

// list 응답 매핑 (도메인에 맞게 조정)
function mapListItem(item) {
  return { ...item, id: item.name, status: item.running ? 'running' : 'stopped' }
}

// 표준 CRUD
export const list<Domain> = async () => {
  const data = await request('GET', `${BASE}/list`)
  return data.map(mapListItem)
}

export const get<Domain> = async (id) => {
  const data = await request('GET', `${BASE}?name=${encodeURIComponent(id)}`)
  return { name: data.name, ...data.config }
}

export const create<Domain> = (data) =>
  request('POST', BASE, data)

export const update<Domain> = (id, data) =>
  request('PUT', `${BASE}?name=${encodeURIComponent(id)}`, data)

export const delete<Domain> = (id) =>
  request('DELETE', `${BASE}?name=${encodeURIComponent(id)}`)
```

**커스텀 엔드포인트**: endpoints 목록에 표준(list/get/create/update/delete) 외 항목이 있으면 추가 함수 생성.
예: `acknowledge` → `export const acknowledge<Domain> = (id) => request('POST', \`${BASE}/acknowledge?name=${encodeURIComponent(id)}\`)`

### Step 3: Hook 생성 (`src/hooks/use<Domain>.js`)

레퍼런스 `useJobs.js` 패턴을 기반으로 생성한다.

**필수 구조:**
```javascript
import { useState, useEffect, useCallback, useRef } from 'react'
import * as <domain>Api from '../api/<domain>'
import { useApp } from '../context/AppContext'

export default function use<Domain>() {
  const [<items>, set<Items>] = useState([])
  const [loading, setLoading] = useState(true)
  const { notify } = useApp()
  const intervalRef = useRef(null)
  const lastErrorRef = useRef(null)

  const fetch<Items> = useCallback(async () => {
    try {
      const data = await <domain>Api.list<Domain>()
      set<Items>(data)
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
    fetch<Items>()
    // polling이 null이면 이 블록 생략
    intervalRef.current = setInterval(fetch<Items>, <polling>)
    return () => clearInterval(intervalRef.current)
  }, [fetch<Items>])

  // toggle 함수 (start/stop 엔드포인트가 있는 경우)
  const toggle<Item> = useCallback(async (item) => {
    try {
      if (item.status === 'running') {
        await <domain>Api.stop<Domain>(item.id)
        notify(`'${item.id}' stopped`, 'success')
      } else {
        await <domain>Api.start<Domain>(item.id)
        notify(`'${item.id}' started`, 'success')
      }
      await fetch<Items>()
    } catch (e) {
      notify(e.reason || e.message, 'error')
    }
  }, [fetch<Items>, notify])

  // remove 함수
  const remove<Item> = useCallback(async (id) => {
    try {
      await <domain>Api.delete<Domain>(id)
      notify(`'${id}' deleted`, 'success')
      await fetch<Items>()
    } catch (e) {
      notify(e.reason || e.message, 'error')
    }
  }, [fetch<Items>, notify])

  return {
    <items>, loading,
    toggle<Item>, remove<Item>,
    refresh: fetch<Items>
  }
}
```

### Step 4: 검증
생성된 코드에 대해 pattern-checker 에이전트의 검사 기준을 자체 적용:
- `request()` import 확인
- `encodeURIComponent` 사용 확인
- `notify` 패턴 확인
- `lastErrorRef` 패턴 확인

### Step 5: 결과 보고
```
## API Layer Generated

### Files Created
- src/api/<domain>.js (N exports)
- src/hooks/use<Domain>.js

### Exports
- API: list, get, create, update, delete, [custom...]
- Hook: <items>, loading, toggle, remove, refresh

### Pattern Check: PASS / FAIL
```
