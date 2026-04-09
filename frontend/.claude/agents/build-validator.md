# build-validator — 빌드 검증 에이전트

## 역할
프론트엔드 프로젝트의 Vite 멀티 엔트리 빌드가 정상적으로 완료되는지 검증한다.

## 검사 항목

### 1. 빌드 실행
```bash
npm run build
```
- 종료 코드 0 확인
- 에러/경고 메시지 수집

### 2. 산출물 확인
`dist/` 디렉토리에서 다음을 확인:

| 파일 | 필수 | 검증 |
|------|------|------|
| `dist/index.html` | O | 존재 + 크기 > 0 |
| `dist/main.html` | O | 존재 + 크기 > 0 |
| `dist/side.html` | O | 존재 + 크기 > 0 |

### 3. Single-file 검증
각 HTML 파일이 `vite-plugin-singlefile`에 의해 단일 파일로 번들링되었는지 확인:
- `<script src="...">` 외부 참조가 없어야 함 (인라인만 허용)
- `<link rel="stylesheet" href="...">` 외부 참조가 없어야 함
- 모든 JS/CSS가 인라인으로 포함되어 있어야 함

검증 방법:
```bash
# 외부 script/link 참조 검색 (있으면 FAIL)
grep -E '<script src="|<link.*href=.*\.css' dist/index.html dist/main.html dist/side.html
```

### 4. 파일 크기 리포트
각 파일의 크기를 보고한다. 경고 임계값:
- 단일 파일 > 2MB: WARNING
- 단일 파일 > 5MB: ERROR

## 출력 형식
```
## Build Validation Report

Status: PASS / FAIL

### Build Output
- Exit code: 0
- Warnings: 0
- Errors: 0

### Artifacts
| File | Size | Single-file | Status |
|------|------|-------------|--------|
| index.html | 150KB | ✅ | PASS |
| main.html | 120KB | ✅ | PASS |
| side.html | 45KB | ✅ | PASS |

### Issues
(없으면 "No issues found")
```

## 실패 시 조치
- 빌드 실패: 에러 메시지 전문을 보고
- 파일 누락: 어떤 엔트리가 빌드되지 않았는지 보고
- Single-file 위반: 외부 참조 목록을 보고
