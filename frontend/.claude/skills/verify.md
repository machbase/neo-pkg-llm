# /verify — 빌드 검증 + 패턴 적합성 체크

## 설명
현재 프론트엔드 프로젝트의 빌드 성공 여부와 코드 패턴 적합성을 검증한다.
`build-validator`와 `pattern-checker` 두 에이전트를 **병렬로** 실행한다.

## 사용법
```
/verify [--build-only] [--pattern-only]
```

- 인자 없음: 빌드 + 패턴 모두 검증
- `--build-only`: 빌드만 검증
- `--pattern-only`: 패턴만 검증

## 실행 절차

### Step 1: 에이전트 병렬 실행

두 Agent를 **동시에** 실행한다:

**Agent 1 — build-validator:**
```
frontend/ 디렉토리에서 npm run build를 실행하여 3개 엔트리(index, main, side) 빌드 성공을 확인한다.
dist/ 디렉토리에 index.html, main.html, side.html 생성 여부와 각 파일이 단일 파일(외부 참조 없음)인지 확인한다.
```

**Agent 2 — pattern-checker:**
```
frontend/src/ 디렉토리의 모든 소스 파일을 검사하여 레퍼런스 패턴 준수 여부를 확인한다.
검사 항목: API 모듈 패턴, Hook 패턴, 컴포넌트 CSS 클래스, BroadcastChannel 프로토콜, 환경변수 prefix.
```

### Step 2: 결과 종합

두 에이전트의 결과를 종합하여 다음 형식으로 보고한다:

```
## Verify Results

### Build ✅ / ❌
- index.html: OK (123KB)
- main.html: OK (98KB)
- side.html: OK (45KB)

### Pattern Check ✅ / ❌
- API modules: 3/3 PASS
- Hooks: 2/2 PASS
- Components: 12/12 PASS
- Issues: (있으면 목록)
```

### Step 3: 자동 수정 (FAIL 시)

패턴 위반이 발견되면:
1. 위반 내용을 사용자에게 보고
2. 자동 수정 가능한 항목은 수정 제안
3. 사용자 확인 후 수정 적용
