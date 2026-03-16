# a2a-sentinel 아키텍처

[English](../ARCHITECTURE.md) | **한국어**

a2a-sentinel은 Go로 작성된 경량 보안 우선 A2A (Agent-to-Agent) 프로토콜 게이트웨이입니다. 이 문서는 시스템 아키텍처, 컴포넌트, 요청 흐름, 설계 원칙을 설명합니다.

**목차:**
1. [고수준 개요](#고수준-개요)
2. [컴포넌트 개요](#컴포넌트-개요)
3. [보안 파이프라인](#보안-파이프라인)
4. [MCP 서버](#mcp-서버)
5. [설계 원칙](#설계-원칙)
6. [그레이스풀 셧다운](#그레이스풀-셧다운)

---

## 고수준 개요

### 시스템 다이어그램

```
┌──────────────────────────┐   ┌──────────────────────────┐
│   HTTP/SSE Client (:8080)│   │   gRPC Client (:8443)    │
└────────────┬─────────────┘   └────────────┬─────────────┘
             │                              │
             ▼                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                  Sentinel Security Gateway                      │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ 1. Protocol Detection                                      │ │
│  │    (JSON-RPC vs REST vs SSE vs gRPC)                       │ │
│  └────────────────────────────────────────────────────────────┘ │
│                               │                                 │
│                               ▼                                 │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ 2. Security Pipeline (2-Layer)                             │ │
│  │                                                            │ │
│  │   Pre-Auth:  Global Rate Limit → IP Rate Limit             │ │
│  │   Auth:      JWT / API Key / Passthrough                   │ │
│  │   Post-Auth: User Rate Limit                               │ │
│  └────────────────────────────────────────────────────────────┘ │
│                               │                                 │
│                               ▼                                 │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ 3. PolicyGuard (ABAC)                                      │ │
│  │    IP, user, agent, method, time, header rule evaluation   │ │
│  └────────────────────────────────────────────────────────────┘ │
│                               │                                 │
│                               ▼                                 │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ 4. Router                                                  │ │
│  │    (Path-prefix or Single agent routing)                   │ │
│  └────────────────────────────────────────────────────────────┘ │
│                               │                                 │
│                               ▼                                 │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ 5. Proxy                                                   │ │
│  │    HTTP / SSE / gRPC (no httputil.ReverseProxy)            │ │
│  │    gRPC ↔ JSON-RPC translation for backend agents          │ │
│  └────────────────────────────────────────────────────────────┘ │
│                               │                                 │
│                               ▼                                 │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ 6. Audit Logging (OTel-compatible) + Prometheus Metrics    │ │
│  │    All decisions recorded with structured fields           │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                 │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ Config Hot-Reload (SIGHUP + fsnotify)                      │ │
│  │    Validate → Diff → Atomic Swap → Notify components       │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│                Backend Agent(s)                                 │
│  (echo, streaming, or any A2A-compliant service)                │
│  (always HTTP — gRPC translation handled by sentinel)           │
└─────────────────────────────────────────────────────────────────┘
```

---

## 컴포넌트 개요

### Protocol Detector (`internal/protocol/`)

들어오는 요청을 분석하여 프로토콜 유형을 결정합니다:

- **JSON-RPC**: `Content-Type: application/json` + `jsonrpc` 필드가 있는 POST 요청
- **SSE**: `Accept: text/event-stream`이 있는 POST 요청
- **REST**: JSON-RPC가 아닌 일반 HTTP 요청
- **Agent Card**: `/.well-known/agent.json` 경로에 대한 GET 요청

`InspectAndRewind` 패턴을 사용하여 바디를 소비하지 않고 읽습니다. 스트리밍 요청은 바디 검사를 건너뜁니다.

### Security Middleware (`internal/security/`)

2계층 보안 파이프라인:

**1계층 — 사전 인증 (Pre-Auth):**
- **Global Rate Limiter**: listen 포트에서 전체 요청 수 제한
- **IP Rate Limiter**: 클라이언트 IP별 요청 수 제한. `trusted_proxies` 설정을 인식하여 올바른 클라이언트 IP 추출

**2계층 — 사후 인증 (Post-Auth):**
- **Authentication**: 설정된 모드에 따라 자격 증명 검증 (JWT, API Key, passthrough)
- **User Rate Limiter**: 인증된 subject별 요청 수 제한. 에이전트별 속도 제한 오버라이드 지원

### Policy Engine (`internal/security/policy/`)

ABAC (Attribute-Based Access Control) 엔진으로 우선순위 정렬 규칙을 평가합니다:

- **IP 조건**: CIDR 범위별 허용/차단
- **사용자 조건**: subject 값별 허용/차단
- **에이전트 조건**: 에이전트 이름별 허용/차단
- **메서드 조건**: A2A 메서드별 허용/차단 (예: `message/send`)
- **시간 조건**: 허용된 시간 범위 외 요청 차단 (타임존 지원)
- **헤더 조건**: 특정 헤더 값에 따른 허용/차단

모든 규칙은 config 리로드 시 원자적으로 교체됩니다.

### Router (`internal/router/`)

두 가지 라우팅 모드를 지원합니다:

- **path-prefix**: URL 경로에서 에이전트 이름 추출 (`/agents/{name}/`)
- **single**: 모든 트래픽을 단일 기본 에이전트로 라우팅

Agent Card Manager를 통해 에이전트 헬스를 확인하고, 비정상 에이전트로의 요청에 503을 반환합니다.

### Proxy (`internal/proxy/`)

`httputil.ReverseProxy`를 사용하지 않고 직접 구현합니다:

- **HTTP Proxy**: 표준 요청-응답 포워딩
- **SSE Proxy**: 각 스트림에 전용 고루틴 유지, 청크를 클라이언트에 전달
- **gRPC Proxy**: 별도 포트의 gRPC 요청을 JSON-RPC로 변환하여 백엔드에 전달

모든 프록시는 hop-by-hop 헤더(Connection, Keep-Alive, Transfer-Encoding 등)를 제거합니다. sentinel 전용 헤더는 절대 백엔드에 주입하지 않습니다.

### Agent Card Manager (`internal/agentcard/`)

- 설정 가능한 간격으로 각 에이전트의 `/.well-known/agent.json` 폴링
- 응답을 메모리에 캐시, 변경 감지 및 알림
- 모든 백엔드의 스킬을 `/agents/.well-known/agent.json`에 집계된 카드로 병합
- `card_change_policy: reject`로 설정 시 변경된 카드 거부
- 설정된 경우 JWK 공개키로 JWS 서명 검증

### Audit Logger (`internal/audit/`)

모든 요청 결정을 OTel 호환 구조화 JSON으로 기록합니다:

```json
{
  "timestamp": "2025-02-26T12:34:56Z",
  "level": "info",
  "msg": "request_decision",
  "http.method": "POST",
  "http.target": "/agents/echo/",
  "a2a.agent.name": "echo",
  "a2a.auth.subject": "user-123",
  "a2a.decision": "allow",
  "a2a.decision.reason": "rate_limit_ok",
  "a2a.rate_limit.user.remaining": 95,
  "a2a.rate_limit.user.reset_secs": 59
}
```

설정 가능한 샘플링:
- 차단 결정: 기본값 100% 기록
- 허용 결정: 기본값 1% 기록 (대용량 환경에서 노이즈 감소)

---

## 보안 파이프라인

요청이 sentinel을 통과하는 전체 순서:

```
1. 클라이언트 연결 수락
   → LimitedListener: max_connections 초과 시 연결 거부
   → trusted_proxies 인식으로 실제 클라이언트 IP 추출

2. Global Rate Limiter (Pre-Auth)
   → 모든 클라이언트 전체 초당 요청 수 제한
   → 초과 시: 429 (hint + docs_url 포함)

3. IP Rate Limiter (Pre-Auth)
   → 클라이언트 IP별 초당 요청 수 제한
   → 초과 시: 429

4. Protocol Detector
   → InspectAndRewind로 바디 읽기 (소비하지 않음)
   → context에 프로토콜 유형 설정

5. Authentication Middleware
   → 설정된 모드로 자격 증명 검증
   → 성공: context에 AuthInfo(subject, verified) 주입
   → 실패: 401/403 (교육적 에러 메시지)

6. User Rate Limiter (Post-Auth)
   → 인증된 subject별 초당 요청 수 제한
   → 에이전트별 오버라이드 지원
   → 초과 시: 429

7. Policy Engine (ABAC)
   → 우선순위 순서로 모든 정책 규칙 평가
   → deny 규칙 매칭: 403 (정책 이름과 이유 포함)
   → 모든 규칙 통과: 계속 진행

8. Router
   → 에이전트 이름 결정 (path-prefix 또는 single 모드)
   → Agent Card Manager로 에이전트 헬스 확인
   → 비정상: 503

9. Proxy
   → SSE: SSEProxy로 스트리밍 처리
   → JSON-RPC/REST: HTTPProxy로 요청-응답 처리
   → hop-by-hop 헤더 제거
   → sentinel 전용 헤더 주입 금지
   → X-Forwarded-For, X-Forwarded-Proto 설정

10. Audit Logger
    → 모든 결정(허용/차단) 기록
    → Prometheus 카운터 업데이트
```

---

## MCP 서버

MCP 서버는 localhost 전용 관리 인터페이스로, MCP 2025-11-25 Streamable HTTP 프로토콜을 구현합니다.

### 3단계 인증 모델

MCP 서버는 세 가지 상태의 인증을 지원합니다:

| 상태 | 조건 | 허용 도구 |
|------|------|-----------|
| `anonymous` | 토큰 없음, `allow_unauthenticated: true` | 읽기 전용 도구 |
| `authenticated` | 유효한 Bearer 토큰 | 모든 도구 (읽기 + 쓰기) |
| `reject` | 토큰 없음, `allow_unauthenticated: false` | 없음 (401 반환) |

### 읽기 도구 (9개)

- `list_agents`: 등록된 모든 에이전트와 헬스 상태 조회
- `health_check`: 특정 에이전트의 헬스 상태 확인
- `get_blocked_requests`: 차단된 요청 목록과 이유 조회
- `get_agent_card`: 특정 에이전트의 Agent Card 조회
- `get_aggregated_card`: 모든 백엔드를 병합한 집계 Agent Card 조회
- `get_rate_limit_status`: 현재 속도 제한 상태 조회
- `list_policies`: 설정된 모든 ABAC 정책 조회
- `evaluate_policy`: 특정 요청 속성에 대한 정책 평가 시뮬레이션
- `list_pending_changes`: 승인 대기 중인 Agent Card 변경 목록 조회

### 쓰기 도구 (6개)

- `update_rate_limit`: 속도 제한 설정 동적 업데이트
- `register_agent`: 새 에이전트 동적 등록
- `deregister_agent`: 에이전트 등록 해제
- `send_test_message`: 에이전트에 테스트 메시지 전송
- `approve_card_change`: 대기 중인 Agent Card 변경 승인
- `reject_card_change`: 대기 중인 Agent Card 변경 거부

### 리소스 (4개)

MCP 리소스를 통해 실시간 상태 조회:
- `sentinel://agents`: 에이전트 목록
- `sentinel://audit-log`: 최근 감사 로그 항목
- `sentinel://metrics`: Prometheus 메트릭 스냅샷
- `sentinel://config`: 현재 활성 설정 (민감 정보 마스킹)

### 설정

```yaml
mcp:
  enabled: true
  port: 8081               # 기본값: 8081 (항상 127.0.0.1 바인딩)
  auth_token: "secret"     # Bearer 토큰 인증
  allow_unauthenticated: false  # false = 토큰 없으면 거부
```

---

## 설계 원칙

### 1. Zero Agent Dependency (에이전트 의존성 없음)

sentinel은 백엔드 에이전트 요청에 어떠한 sentinel 전용 헤더나 메타데이터도 주입하지 않습니다. 에이전트는 직접 호출되는지 sentinel을 통해 호출되는지 구분할 수 없어야 합니다.

**금지 사항:**
- `X-Sentinel-*` 헤더
- Agent Card의 sentinel 전용 필드
- 백엔드에 대한 프로토콜 수정

### 2. Security ON by Default (기본적으로 보안 활성화)

모든 보안 기능은 기본적으로 활성화됩니다. 보호를 비활성화하려면 명시적인 설정이 필요합니다.

- 기본 인증 모드: `passthrough-strict` (헤더 필요)
- 기본 속도 제한: 활성화됨
- 기본 replay 감지: 경고 모드
- 기본 SSRF 방어: 활성화됨

### 3. Educational Errors (교육적 에러)

모든 에러 응답은 `hint`와 `docs_url`을 포함합니다:

```json
{
  "error": {
    "code": -32001,
    "message": "Authentication required",
    "hint": "Include 'Authorization: Bearer <token>' header. For development, set auth.mode: passthrough.",
    "docs_url": "https://a2a-sentinel.dev/docs/auth"
  }
}
```

### 4. OTel-Compatible Audit Logs (OTel 호환 감사 로그)

필드 이름은 OpenTelemetry 시맨틱 컨벤션을 따릅니다:
- `http.method`, `http.target`, `http.status_code`
- `a2a.agent.name`, `a2a.auth.subject`, `a2a.decision`
- `a2a.rate_limit.user.remaining`, `a2a.rate_limit.ip.remaining`

### 5. No httputil.ReverseProxy (ReverseProxy 미사용)

sentinel은 `net/http/httputil.ReverseProxy`를 사용하지 않습니다. 직접 구현하는 이유:
- hop-by-hop 헤더 완전 제어
- SSE 스트리밍 동작 정밀 제어
- sentinel 전용 헤더 주입 방지 보장
- gRPC ↔ JSON-RPC 변환 통합

### 6. Explicit Body Inspection (명시적 바디 검사)

`InspectAndRewind` 패턴: 바디를 읽고 되감아 다운스트림에서 다시 읽을 수 있도록 합니다. 스트리밍 요청(SSE)은 바디 검사를 건너뜁니다.

---

## 그레이스풀 셧다운

sentinel은 SIGTERM 또는 SIGINT 수신 시 순서대로 종료합니다:

```
1. 새 연결 수락 중지 (LimitedListener 닫기)
2. /readyz를 즉시 "not ready"로 전환
3. 진행 중인 HTTP 요청 완료 대기 (최대 shutdown.timeout)
4. 활성 SSE 스트림 드레이닝 대기 (최대 shutdown.drain_timeout)
5. Agent Card Manager 폴링 중지
6. MCP 서버 종료
7. gRPC 서버 종료
8. 감사 로거 플러시 및 닫기
9. 종료
```

Kubernetes Pod 종료와 호환됩니다: `preStop` 훅 없이 `terminationGracePeriodSeconds`를 `shutdown.timeout` + `shutdown.drain_timeout` 합산값보다 크게 설정하세요.

---

더 자세한 내용은 영어 원본 문서 [ARCHITECTURE.md](../ARCHITECTURE.md)를 참조하세요.
