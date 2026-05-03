# 보안 정책

<p>
  <a href="./SECURITY.md"><img alt="English security policy" src="https://img.shields.io/badge/lang-English-6b7280.svg?style=flat-square"></a>
  <a href="./SECURITY-ko.md"><img alt="Korean security policy" src="https://img.shields.io/badge/lang-%ED%95%9C%EA%B5%AD%EC%96%B4-2563eb.svg?style=flat-square"></a>
</p>

Pindoc은 project memory, code reference, agent-written artifact, 선택적
identity-provider 설정을 저장하는 self-hosted software입니다. Pindoc
인스턴스는 해당 프로젝트의 민감한 인프라로 취급해야 합니다.

## 지원 버전

아직 stable release는 없습니다. 첫 tagged release series가 생기기 전까지
보안 수정은 `main`을 대상으로 합니다. 이후 이 문서에 지원 branch를 명시합니다.

## 취약점 제보

가능하면 GitHub Security Advisories를 사용하세요. private advisory가 아직
켜져 있지 않다면 exploit detail, secret, token, private URL, 민감한 project
data를 포함하지 않는 최소 공개 issue를 열고 maintainer에게 private contact
path를 요청하세요.

포함하면 좋은 정보:

- 영향받는 commit 또는 version,
- 배포 형태: loopback-only, LAN, public reverse proxy, hosted demo,
- `PINDOC_AUTH_PROVIDERS` 또는 `PINDOC_ALLOW_PUBLIC_UNAUTHENTICATED` 설정 여부,
- 안전하게 공유 가능한 최소 재현 절차.

## 로컬 신뢰 모델

기본 Docker 설정은 단일 operator의 loopback 사용을 전제로 합니다.

- `PINDOC_BIND_ADDR=127.0.0.1:5830`
- `PINDOC_AUTH_PROVIDERS` empty
- `PINDOC_ALLOW_PUBLIC_UNAUTHENTICATED=false`
- Docker는 daemon을 `127.0.0.1:${PINDOC_DAEMON_PORT}`에 publish합니다.

Loopback request는 신뢰하고 local owner identity로 매핑합니다. 이것은 로컬
agent workflow를 위한 의도적인 선택이며 public internet security model이
아닙니다.

## 외부 노출

`PINDOC_BIND_ADDR`가 non-loopback이면 다음 중 하나가 참이 아닐 경우 Pindoc은
시작을 거부합니다.

- `PINDOC_AUTH_PROVIDERS`가 `github` 같은 identity provider를 활성화, 또는
- `PINDOC_ALLOW_PUBLIC_UNAUTHENTICATED=true`로 인증 없는 network model을 명시적으로 opt in.

쓰기 가능한 public internet 배포에서 `PINDOC_ALLOW_PUBLIC_UNAUTHENTICATED=true`를
사용하지 마세요. 신뢰된 LAN 또는 추가 access control이 있는 reverse proxy 뒤에서만
사용해야 합니다.

Pindoc HTTP daemon은 cross-origin browser request를 기본 거부합니다.
신뢰된 frontend가 다른 origin에서 daemon을 호출해야 할 때만
`PINDOC_ALLOWED_ORIGINS`에 콤마 구분 allowlist를 설정하세요.
`PINDOC_DEV_MODE=true`는 로컬 도구용 wildcard CORS이며 공개 인스턴스에서
사용하면 안 됩니다. Daemon은 baseline security header와 asset blob hardened
CSP도 직접 부착하므로 reverse proxy가 응답을 재작성한다면 동등하거나 더 엄격한
header를 유지해야 합니다.

## Read-only 공개 데모

공개 데모는 일반적인 writable daemon을 그대로 노출하면 안 됩니다. 권장 형태:

- Pindoc daemon은 private으로 두거나 reverse proxy 뒤에 둡니다.
- public internet에서 `/mcp`를 차단합니다.
- public non-`GET`/`HEAD`/`OPTIONS` method를 차단합니다.
- project creation, provider admin, invite/member management, task metadata write, inbox review, read event, onboarding identity, join 같은 admin/mutation route를 차단합니다.
- 모든 참조 repo가 public이고 demo owner가 source-code browsing을 명시 승인한 경우가 아니면 git preview endpoint를 차단합니다.

운영 체크리스트는 [공개 데모 운영안](docs/22-public-demo-ko.md)을 참고하세요.

## Secret과 민감 정보

데모 공개 전 다음을 scrub하거나 제외하세요.

- API token, OAuth client secret, signing key, database URL,
- local username, home-directory path, private machine name, internal IP,
- private repo name, branch name, deployment host, 미공개 customer/project name,
- private chat log 또는 issue tracker를 인용한 artifact body,
- private repository의 git blob/diff preview route.

Public Reader는 공개용으로 큐레이션된 data set일 때만 안전합니다. Reverse proxy의
read-only 제어는 이미 저장된 artifact content를 sanitize하지 않습니다.

## Dependency와 Build 보안

Docker image는 web UI와 Go server를 source에서 build하고 non-root `pindoc`
user로 실행하며 runtime cache를 `/var/lib/pindoc/cache`에 저장합니다. 운영자는
production image tag를 pin하고 base image를 최신으로 유지하며, public 노출
container에 writable source tree를 mount하지 않아야 합니다.
