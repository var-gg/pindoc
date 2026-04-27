@PINDOC.md

# Codex Local Notes

## VARGG

- Canonical repo: `A:\vargg-workspace\vargg`
- Legacy source repos under `A:\vargg-workspace\10_frontend\vargg-frontend` and `A:\vargg-workspace\20_backend\vargg-backend` are reference-only unless a task explicitly calls for backport or rollback investigation
- Current production path is GCE in project `var-gg`, not App Engine
- Current deployment model is `prebaked base image + app-only MIG rollout`
- Primary public hosts are `var.gg`, `www.var.gg` (Cloudflare 경유)
- `api.var.gg`는 폐기됨 - FE->BE는 내부 네트워크 통신
- Legacy `curioustore.com`, `www.curioustore.com` -> GCE nginx에서 301 redirect to var.gg
- 인프라: GCE + Cloudflare (Free) - GCP LB/Cloud Armor 제거됨

## 로컬 시크릿

- 위치: `~/.claude/secrets.env`
- 네이밍: `{프로젝트}_{서비스}_{용도}` (예: `VARGG_CF_DNS_TOKEN`)
- API 토큰이 필요하면 이 파일에서 읽어서 사용

<!-- pindoc:register-separation:v1 BEGIN -->
### Register 분리 - 사용자 대화와 Pindoc artifact

Pindoc artifact 본문(Context / Decision / Rationale / Alternatives / Consequences 같은 섹션)은 구조화된 register에 속해서 표, bullet, ADR 스타일 축약이 자연스럽다. 반면 사용자 대면 응답은 추론 흐름이 연결된 산문 register에 속한다. "Alt A(...) · B(...)" 같은 축약, 괄호 안 한 단어 기각 이유, 중점으로 나열한 짧은 구절이 artifact 본문에서 대화로 역류하지 않게 한다. 애매할 때는 응답을 두세 문장 산문으로 다시 쓰며 추론을 압축이 아닌 서술로 노출한다.
<!-- pindoc:register-separation:v1 END -->
