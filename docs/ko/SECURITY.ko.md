# 보안 가이드 — a2a-sentinel

[English](../SECURITY.md) | **한국어**

**a2a-sentinel은 "기본적으로 보안 활성화(Security ON by Default)" 원칙으로 구축되었습니다.**

모든 보안 기능은 기본적으로 활성화됩니다. 보호를 비활성화하려면 명시적인 설정이 필요합니다. 이 가이드는 위협 모델, 인증 모드, 속도 제한 전략, 그리고 보안 요구사항에 맞게 sentinel을 설정하는 방법을 설명합니다.

---

## 목차

1. [보안 철학](#보안-철학)
2. [위협 모델 및 방어](#위협-모델-및-방어)
3. [인증 모드](#인증-모드)
4. [속도 제한 (2계층)](#속도-제한-2계층)
5. [Policy Engine (ABAC)](#policy-engine-abac)
6. [Agent Card 보안](#agent-card-보안)
7. [MCP 서버 보안](#mcp-서버-보안)
8. [감사 로깅](#감사-로깅)
9. [푸시 알림 보호](#푸시-알림-보호)
10. [재전송 공격 방지](#재전송-공격-방지)
11. [에러 메시지 및 힌트](#에러-메시지-및-힌트)

---

## 보안 철학

a2a-sentinel은 합리적인 기본값을 가진 **심층 방어(defense-in-depth)** 접근 방식을 따릅니다:

- **명시적 > 암묵적**: 모든 보안 결정은 명시적입니다. 모든 결정(허용/차단)을 로깅합니다.
- **교육적 에러**: 모든 보안 차단에는 무엇이 잘못되었는지 설명하는 `hint`와 수정 방법을 알려주는 `docs_url`이 포함됩니다.
- **관찰 가능성**: 모든 결정이 감사 추적을 위해 OTel 호환 구조화 형식으로 로깅됩니다.
- **게이트웨이 책임**: sentinel이 에이전트를 보호합니다. 에이전트는 sentinel 전용 요구사항을 검증할 필요가 없습니다.

### 보안 수준

| 수준 | 사용 사례 | Config 프로파일 |
|------|-----------|-----------------|
| **개발** | 로컬 테스트, 인증 불필요 | `sentinel init --profile dev` |
| **엄격한 개발** | 팀 테스트, 인증 헤더 필요하지만 검증하지 않음 | `sentinel init --profile strict-dev` |
| **프로덕션** | 전체 JWT 검증, 적극적인 속도 제한 | `sentinel init --profile prod` |

---

## 위협 모델 및 방어

실제 위협과 a2a-sentinel 방어의 매핑:

| # | 위협 | 공격 벡터 | Sentinel 방어 | 설정 |
|---|------|-----------|---------------|------|
| 1 | **무단 접근** | 인증 토큰 없음 또는 위조 | 2계층 인증 (passthrough-strict 기본값) | `security.auth.mode` |
| 2 | **DoS/DDoS** | 단일 IP에서 요청 폭주 | IP별 속도 제한 (사전 인증) | `security.rate_limit.ip.per_ip`, `listen.global_rate_limit` |
| 3 | **사용자 남용** | 인증된 단일 사용자의 게이트웨이 과부하 | 사용자별 속도 제한 (사후 인증) | `security.rate_limit.user.per_user` |
| 4 | **Agent Card 오염** | 공격자가 전송 중 Agent Card 수정 | 변경 감지 + 알림 로깅 | `agents[].card_change_policy` |
| 5 | **캐시 오염** | 공격자가 폴링 중 악성 카드 주입 | JWS 서명 검증 | `security.card_signature.require` |
| 6 | **푸시 알림을 통한 SSRF** | 공격자가 게이트웨이를 통해 내부 네트워크 접근 | URL 검증, 사설 IP 차단, HTTPS 강제 | `security.push.block_private_networks` |
| 7 | **재전송 공격** | 공격자가 이전 요청을 재전송하여 작업 트리거 | Nonce + 타임스탬프 검증 (경고/필수 정책) | `security.replay.enabled` |
| 8 | **중간자 공격** | 에이전트와의 암호화되지 않은 통신 | 기본 TLS 강제 | `agents[].allow_insecure: false` |
| 9 | **리소스 고갈** | 에이전트당 너무 많은 동시 SSE 스트림 | 에이전트별 스트림 제한 | `agents[].max_streams` |
| 10 | **연결 고갈** | 게이트웨이 전체 연결 과다 | 전역 연결 제한 | `listen.max_connections` |
| 11 | **무단 에이전트 접근** | 사용자가 제한된 에이전트나 메서드에 접근 | 속성 기반 규칙을 사용하는 ABAC 정책 엔진 | `security.policies[]` |
| 12 | **비업무 시간 공격** | 모니터링되지 않는 시간에 공격 | 시간 기반 정책 제한 | `security.policies[].conditions.time` |

---

## 인증 모드

a2a-sentinel은 `security.auth.mode`로 제어되는 네 가지 인증 모드를 지원합니다.

### 1. passthrough (개발 전용)

**동작**: Authorization 헤더 유무에 상관없이 요청을 허용합니다. 검증 없음.

**사용 사례**: 에이전트가 준비되기 전의 로컬 개발.

```yaml
security:
  auth:
    mode: passthrough
```

**위험**: 전혀 보호되지 않습니다. localhost에서만 안전합니다.

---

### 2. passthrough-strict (기본값)

**동작**: Authorization 헤더를 요구하지만 토큰을 검증하지는 않습니다. subject 클레임을 추출하고 "unverified:" 접두사와 함께 로깅합니다.

**사용 사례**: 팀 개발, docker-compose 테스트, JWT 오버헤드 없이 엄격한 헤더 강제.

```yaml
security:
  auth:
    mode: passthrough-strict
    allow_unauthenticated: false  # 헤더 필수
```

**동작 방식**:
1. Authorization 헤더 없이 요청 도착 → **401로 거부**
2. Authorization 헤더가 있는 요청 → 허용, JWT인 경우 subject 추출 (불투명 토큰은 잘라서 기록)
3. Subject가 `unverified:<subject>`로 로깅됨 (미검증 출처 표시)

**감사 로그 예시**:
```json
{
  "timestamp": "2025-02-26T12:34:56Z",
  "a2a.auth.subject": "unverified:user-123"
}
```

---

### 3. jwt (프로덕션)

**동작**: 전체 JWT 검증 — issuer, audience, 만료, JWKS 서명 검증.

**사용 사례**: OAuth2/OIDC 토큰 프로바이더를 사용하는 프로덕션.

```yaml
security:
  auth:
    mode: jwt
    allow_unauthenticated: false
    schemes:
      - type: bearer
        jwt:
          issuer: https://auth.example.com
          audience: sentinel-api
          jwks_url: https://auth.example.com/.well-known/jwks.json
```

**검증 항목**:
- 토큰 형식: `Authorization: Bearer <JWT>`
- JWKS 엔드포인트에 대한 서명 검증
- 클레임 검증: `iss`, `aud`, `exp`
- Subject (`sub` 클레임) 추출 및 검증됨으로 로깅

---

### 4. api-key (단순 프로덕션)

**동작**: 단일 공유 시크릿을 사용한 간단한 API 키 인증.

**사용 사례**: 복잡한 토큰 인프라 없는 내부 서비스.

```yaml
security:
  auth:
    mode: api-key
    api_key:
      header: X-API-Key
      value: your-secret-key
```

---

### 5. none (신뢰할 수 있는 프록시 전용)

**동작**: 인증 없음. 모든 요청을 허용합니다.

**사용 사례**: 인증을 처리하는 신뢰할 수 있는 프록시 뒤의 내부 네트워크.

```yaml
security:
  auth:
    mode: none
```

**경고**: 신뢰할 수 있는 네트워크에서만 사용하세요. sentinel은 인증 없이 모든 요청을 통과시킵니다.

---

## 속도 제한 (2계층)

### 1계층: 사전 인증 속도 제한 (IP별)

인증 전에 실행되어 비용이 많이 드는 인증 작업 전에 과도한 요청을 차단합니다.

```yaml
listen:
  global_rate_limit: 5000    # 전역 초당 요청 수 (모든 클라이언트 합산)

security:
  rate_limit:
    ip:
      per_ip: 100            # IP별 초당 요청 수
      burst: 200             # 버스트 허용량
```

### 2계층: 사후 인증 속도 제한 (사용자별)

인증 이후 실행되어 인증된 사용자의 남용을 방지합니다.

```yaml
security:
  rate_limit:
    user:
      per_user: 100          # 사용자별 분당 요청 수
      burst: 50              # 버스트 허용량
    enabled: true
```

**에이전트별 오버라이드:**
```yaml
agents:
  - name: premium-agent
    rate_limit:
      per_user: 1000         # 이 에이전트의 사용자별 더 높은 제한
```

### 속도 제한 에러 응답

두 계층 모두 남은 대기 시간과 힌트를 포함한 429를 반환합니다:

```json
{
  "error": {
    "code": 429,
    "message": "Rate limit exceeded",
    "hint": "Current limit: 100 req/min. Wait 30s or contact admin.",
    "docs_url": "https://a2a-sentinel.dev/docs/rate-limit"
  }
}
```

---

## Policy Engine (ABAC)

ABAC (Attribute-Based Access Control) 엔진은 속성 기반의 세밀한 접근 제어를 제공합니다. 규칙은 우선순위 순서로 평가됩니다 (낮은 숫자 = 높은 우선순위).

### 정책 구조

```yaml
security:
  policies:
    - name: 정책-이름
      priority: 10           # 낮을수록 먼저 평가
      effect: deny           # deny 또는 allow
      conditions:
        # 하나 이상의 조건
```

### 조건 유형

**IP 기반:**
```yaml
conditions:
  source_ip:
    cidr: ["192.168.0.0/16", "10.0.0.0/8"]
```

**사용자 기반:**
```yaml
conditions:
  user: ["blocked-user@example.com"]        # 특정 사용자 차단
  user_not: ["admin@example.com"]           # admin을 제외한 모두 차단
```

**에이전트 기반:**
```yaml
conditions:
  agent: ["internal-agent"]                 # 특정 에이전트 접근 제한
```

**메서드 기반:**
```yaml
conditions:
  method: ["tasks/cancel"]                  # 특정 A2A 메서드 차단
```

**시간 기반:**
```yaml
conditions:
  time:
    outside: "09:00-17:00"                  # 업무 시간 외 차단
    timezone: "America/New_York"
```

**헤더 기반:**
```yaml
conditions:
  header:
    name: "X-Client-Version"
    value: "1.0"                            # 구형 클라이언트 차단
```

### 정책 예시

```yaml
security:
  policies:
    # 내부 IP 차단
    - name: block-internal-ips
      priority: 10
      effect: deny
      conditions:
        source_ip:
          cidr: ["192.168.0.0/16"]

    # 업무 시간 외 접근 차단
    - name: business-hours-only
      priority: 20
      effect: deny
      conditions:
        time:
          outside: "09:00-17:00"
          timezone: "Asia/Seoul"

    # admin만 내부 에이전트 접근 허용
    - name: restrict-internal-agent
      priority: 30
      effect: deny
      conditions:
        agent: ["internal-agent"]
        user_not: ["admin@example.com"]

    # 특정 메서드 차단
    - name: block-cancel-for-guests
      priority: 40
      effect: deny
      conditions:
        method: ["tasks/cancel"]
        user: ["guest@example.com"]
```

### MCP를 통한 정책 평가

MCP 서버의 `evaluate_policy` 도구로 정책 규칙을 시뮬레이션할 수 있습니다:

```
evaluate_policy({
  "source_ip": "192.168.1.1",
  "user": "test@example.com",
  "agent": "echo",
  "method": "message/send"
})
```

---

## Agent Card 보안

Agent Card는 에이전트 기능을 광고하는 공개 문서입니다. sentinel은 카드의 무결성을 보호합니다.

### 변경 감지

sentinel은 폴링할 때마다 Agent Card 해시를 추적합니다:

```yaml
agents:
  - name: echo
    url: http://echo-agent:9000
    card_change_policy: alert    # alert (기본값) 또는 reject
```

- **alert**: 변경 감지 시 감사 로그에 경고 기록
- **reject**: 변경된 카드 거부, 이전 버전 유지

### MCP 기반 카드 변경 승인 워크플로우

`card_change_policy: approve`로 설정하면 변경이 승인될 때까지 큐에 보관됩니다:

```yaml
agents:
  - name: echo
    card_change_policy: approve
```

MCP 도구로 대기 중인 변경 관리:
- `list_pending_changes`: 승인 대기 중인 변경 목록
- `approve_card_change`: 변경 승인
- `reject_card_change`: 변경 거부

### JWS 서명 검증

Agent Card에 JWS 서명이 있는 경우 sentinel이 검증합니다:

```yaml
security:
  card_signature:
    require: true
    jwk_file: /etc/sentinel/agent-signing.jwk
```

---

## MCP 서버 보안

MCP 서버는 localhost 전용 관리 인터페이스입니다:

- **바인딩**: 항상 `127.0.0.1`만 바인딩 (외부에서 접근 불가)
- **포트**: 기본값 8081 (게이트웨이 포트 8080과 분리)
- **인증**: Bearer 토큰 기반 (3단계 상태 모델)

### 3단계 인증 상태

| 상태 | 조건 | 접근 수준 |
|------|------|-----------|
| `anonymous` | 토큰 없음 + `allow_unauthenticated: true` | 읽기 전용 도구만 |
| `authenticated` | 유효한 Bearer 토큰 | 모든 도구 (읽기 + 쓰기) |
| `reject` | 토큰 없음 + `allow_unauthenticated: false` | 없음 (401 반환) |

### 설정

```yaml
mcp:
  enabled: true
  port: 8081
  auth_token: "your-strong-secret-token"
  allow_unauthenticated: false    # 엄격 모드: 토큰 없으면 모두 거부
```

**보안 권장 사항:**
- 강력한 랜덤 토큰 사용 (최소 32자)
- MCP 포트를 방화벽으로 외부에서 차단
- 프로덕션에서 `allow_unauthenticated: false` 유지

---

## 감사 로깅

모든 보안 결정은 OTel 호환 구조화 JSON으로 기록됩니다.

### 로그 필드

```json
{
  "timestamp": "2025-02-26T12:34:56Z",
  "level": "info",
  "msg": "request_decision",
  "http.method": "POST",
  "http.target": "/agents/echo/",
  "a2a.agent.name": "echo",
  "a2a.auth.subject": "user-123",
  "a2a.auth.verified": true,
  "a2a.decision": "allow",
  "a2a.decision.reason": "rate_limit_ok",
  "a2a.rate_limit.user.remaining": 95,
  "a2a.rate_limit.user.reset_secs": 59,
  "net.peer.ip": "203.0.113.1"
}
```

### 차단 결정 예시

```json
{
  "timestamp": "2025-02-26T12:35:00Z",
  "level": "warn",
  "msg": "request_decision",
  "a2a.decision": "block",
  "a2a.decision.reason": "rate_limit_exceeded",
  "a2a.rate_limit.user.remaining": 0,
  "a2a.rate_limit.user.reset_secs": 45
}
```

### 샘플링 설정

```yaml
logging:
  audit:
    sample_rate_allow: 0.01      # 허용 결정의 1% 기록
    sample_rate_block: 1.0       # 차단 결정은 100% 기록
    max_body_bytes: 512          # 로깅할 최대 바디 크기
```

---

## 푸시 알림 보호

A2A Push Notification을 사용할 경우, sentinel은 SSRF (Server-Side Request Forgery) 공격을 방어합니다.

### SSRF 방어

```yaml
security:
  push:
    block_private_networks: true     # 사설 IP 범위 차단
    require_https: true              # HTTPS 강제
    allowed_domains:                 # 허용할 도메인 화이트리스트
      - "*.example.com"
      - "notifications.partner.com"
    dns_fail_policy: block           # DNS 실패 시 차단 (또는 allow)
```

**차단되는 IP 범위:**
- `10.0.0.0/8` — 사설 네트워크
- `172.16.0.0/12` — 사설 네트워크
- `192.168.0.0/16` — 사설 네트워크
- `127.0.0.0/8` — 루프백
- `169.254.0.0/16` — 링크 로컬 (클라우드 메타데이터 서비스)

---

## 재전송 공격 방지

sentinel은 nonce와 타임스탬프를 사용하여 재전송 공격을 방지합니다.

### 설정

```yaml
security:
  replay:
    enabled: true
    policy: require              # warn (경고만) 또는 require (필수)
    window: 300s                 # nonce 유효 기간 (5분)
    clock_skew: 30s              # 허용되는 시계 편차
    nonce_source: header         # header 또는 body에서 nonce 추출
    storage: memory              # memory 또는 redis
```

**warn 모드**: nonce 없거나 재사용된 경우 감사 로그에 경고만 기록하고 요청은 통과
**require 모드**: nonce 없거나 재사용된 경우 요청 거부 (403)

---

## 에러 메시지 및 힌트

sentinel의 모든 에러 응답은 개발자가 문제를 빠르게 해결할 수 있도록 설계되었습니다.

### JSON-RPC 에러 형식

```json
{
  "jsonrpc": "2.0",
  "id": "1",
  "error": {
    "code": -32001,
    "message": "Authentication required",
    "data": {
      "hint": "Include 'Authorization: Bearer <token>' header. For development, set auth.mode: passthrough.",
      "docs_url": "https://a2a-sentinel.dev/docs/auth",
      "sentinel_code": "AUTH_MISSING"
    }
  }
}
```

### HTTP REST 에러 형식

```json
{
  "error": {
    "code": 401,
    "message": "Authentication required",
    "hint": "Include 'Authorization: Bearer <token>' header.",
    "docs_url": "https://a2a-sentinel.dev/docs/auth"
  }
}
```

### 일반적인 에러 코드

| sentinel_code | 상태 | 설명 |
|---------------|------|------|
| `AUTH_MISSING` | 401 | Authorization 헤더 없음 |
| `AUTH_INVALID` | 401 | 유효하지 않은 토큰 |
| `AUTH_EXPIRED` | 401 | 만료된 JWT |
| `RATE_LIMIT_IP` | 429 | IP 속도 제한 초과 |
| `RATE_LIMIT_USER` | 429 | 사용자 속도 제한 초과 |
| `POLICY_DENIED` | 403 | ABAC 정책에 의해 차단 |
| `AGENT_UNHEALTHY` | 503 | 대상 에이전트 비정상 |
| `REPLAY_DETECTED` | 403 | 재전송 공격 감지 |
| `SSRF_BLOCKED` | 403 | SSRF 공격 차단 |

---

더 자세한 내용은 영어 원본 문서 [SECURITY.md](../SECURITY.md)를 참조하세요.
