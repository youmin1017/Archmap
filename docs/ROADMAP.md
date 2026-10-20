# Archmap Roadmap（M7 之後）

> 建議放在 `docs/ROADMAP.md`。與 `PROJECT_BRIEF.md`、`DECISIONS.md`、`ARCHITECTURE.md` 並列。
> 若本文件與 `DECISIONS.md` 衝突，以 `DECISIONS.md`（實際驗證後的決定）為準。
> 每個里程碑內的 Prompt 為英文，可直接貼給 Claude Code；說明文字為中文。

## 0. 現況與編號說明

目前已完成：

- M0 到 M6 的大部分（巢狀畫布、draft / diff / lint / 3-way merge、版本比較、圖層視圖、受保護元素、角色、部署欄位、自由屬性）。
- **M7 登入**：server-side session、CSRF、本機密碼與 dev 登入 provider，OIDC-ready（ADR-053..061）。
- **M10 模型充實**：type、owner 與繼承、營運欄位、新 lint、「無人管理」視圖（ADR-062..068）。後續擴充：owner 可指向使用者或群組，群組由 maintainer 管理（ADR-069、ADR-070）。
- **M10.1 聚焦式 Canvas**（ADR-071）與 **M10.2 可設定審核政策**（ADR-072），見下方 Phase 2。

尚未完成：Phase 0 的 Playwright E2E 與效能基準、**M8 部署**、**M9 自訂技術圖示上傳**，以及 M11 之後的里程碑。

> 編號調整：先前對話中 `M10.5` 與 `M11a / M11b` 的編號在此重新整理。
> 重要順序變更：**模型充實（Owner、type 等）排在匯入匯出之前**，因為 YAML 格式應該一次包含這些欄位，避免格式反覆改版。
> 插入的里程碑：**M10.1 聚焦式 Canvas** 與 **M10.2 可設定審核政策** 是在 M10 之後依使用回饋加入的。為了不改動後續編號（M11 起已在文件與 ADR 中被引用），以小數編號插入。

## 1. 總覽與順序

```
Phase 0  整頓（不算里程碑，先做）
Phase 1  讓別人能安全使用     M7 登入 → M8 部署 → M9 圖示上傳
Phase 2  讓模型更實用         M10 模型充實 → M10.1 聚焦式 Canvas → M10.2 審核政策 → M11 匯入匯出
Phase 3  讓資料自動進來       M12 連接器框架 + OpenEverest
Phase 4  讓圖「活起來」       M13 即時狀態 → M14 API endpoint 目錄
Phase 5  企業整合             M15 OIDC
Backlog  之後再說
```

依賴關係：

```
M7 ──► M8 ──► (自用 / dogfooding)
M7 ──► M15 (OIDC，建立在 M7 的 provider 介面上)
M10 ──► M11 ──► M12 ──► M13
M10 ──► M12   (人工維護欄位的規則在 M10 決定，M12 的同步必須遵守)
M10.2 ──► M11、M12 (直接編輯模式下，匯入與連接器的產出仍一律是 draft)
M10.1 與 M10.2 互相獨立
M9  獨立，可與 M10 並行
M14 依賴 M11（可匯入 OpenAPI）與 M10（owner 與 type）
```

## 2. 每個里程碑都要遵守（跨里程碑規則）

- 先進 plan mode，列出計畫與未決問題，經我同意再寫程式碼；做完停在 DoD 並總結。
- 變更 API 時同步更新 `contracts/openapi.yaml` 並重新產生 Go 與 TS 程式碼（`task gen`）。
- 非顯而易見的決定追加到 `docs/DECISIONS.md`；文件與程式碼有出入時，先更新文件。
- 測試先行：核心邏輯用 table-driven 測試，資料庫相關用整合測試。
- **不得使用 `--no-verify` 跳過 hooks**；hook 失敗就修正問題。格式化由 pre-commit 自動處理，CI 只檢查不修改。
- **審核政策（M10.2 起）**：使用者在 main 上的直接編輯可以自動合併；但**匯入、連接器、批次工具的產出一律建立 draft 並走審核**，不受 `direct` 政策影響。
- 資料分類要明確：**Model**（走 changeset、版本、review；`direct` 政策下由編輯者直接合併）、**Layout**（不產生版本）、**Runtime status**（唯讀執行期資料，不進 changeset）、**Human-owned fields**（外部同步不得覆蓋）。
- 外部函式庫（Echo v5、oapi-codegen、TanStack Start、OIDC、Prometheus 等）一律先查當前官方文件，不憑記憶寫。
- Seed 與測試資料只用虛構的團隊與聯絡方式（repo 是公開的）。

---

## Phase 0：整頓（開始 M7 前先做）

目標：讓文件、品質與安全狀態追上現實。

| 項目           | 內容                                                                                                                                                                 |
| -------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 文件同步       | `PROJECT_BRIEF.md` 與實際狀況有出入（例如使用 Taskfile 而非 Makefile、已有圖層視圖與屬性欄位）。更新 brief 與 DECISIONS，並更新 `AGENTS.md` / `CLAUDE.md` 的文件索引 |
| 公開 repo 檢查 | 確認 `.env` 未進版控、git 歷史沒有真實密碼或金鑰、`.env.example` 只有預設值                                                                                          |
| E2E 測試       | 用 Playwright 涵蓋主流程：建立 draft → 編輯 → 提交 → 另一位 maintainer 合併 → 查看版本                                                                               |
| 效能基準       | 灌入 500 到 1000 個節點，量測展開收合與 ELK 排版時間，記錄結果與瓶頸                                                                                                 |
| Dogfooding     | 用 Archmap 描述 Archmap 自己（含 Postgres、API、web、部署節點），整理遇到的 UX 問題與欄位缺漏                                                                        |
| Git hooks 確認 | 已有 lefthook。確認 pre-commit 自動格式化、commit-msg 檢查、pre-push 檢查，CI 同步執行相同檢查                                                                       |

**Prompt**

```markdown
Housekeeping before M7. Use plan mode.

1. Reconcile docs/PROJECT_BRIEF.md, ARCHITECTURE.md, DECISIONS.md, README.md and AGENTS.md
   with the current code (e.g. Taskfile vs Makefile, layers views, properties, versions
   of tools). List every discrepancy first.
2. Audit the repo for secrets: check tracked files and git history for credentials; confirm
   .env is ignored.
3. Add Playwright E2E for the core flow: create draft, edit, submit, merge by a different
   maintainer, browse the resulting version. Run it in CI.
4. Add a repeatable performance check with a generated model of ~1000 elements; report
   expand/collapse and layout times and the top bottlenecks (do not optimize yet).
5. Confirm hooks: pre-commit auto-format of staged files, commit-msg convention check,
   pre-push checks, and that CI runs the same checks without modifying files.
   Summarize findings and propose fixes; stop for approval before large changes.
```

---

## Phase 1：讓別人能安全使用

### M7 登入（OIDC-ready，但不實作 OIDC）

**目標**：取代 dev header，並讓日後加 OIDC 只是新增 provider。

範圍：

- `users` 只放內部身分；憑證放 `user_identities`（provider、issuer、subject）。外部身分以 `(issuer, subject)` 識別，**不以 email**。
- Auth Provider 介面；實作：本機 email + argon2id、dev header（僅開發環境，其他環境啟動時直接失敗）。
- Server-side session、HttpOnly cookie、CSRF 防護、登入限速、停用使用者即時生效、登出。
- 角色來源可替換（先存在使用者上，預留 claims / 群組對應的擴充點）。
- 不以 email 自動合併帳號；連結身分為 maintainer 的明確動作。
- 保留本機 break-glass maintainer；可設定「SSO 啟用後關閉一般使用者的密碼登入」。
- Maintainer 使用者管理（建立、停用、重設密碼、改角色、檢視與解除身分連結）。

DoD：以假 provider 測試以下情境皆通過：新使用者、既有使用者、被停用、同 email 不同 subject（不得連結）、角色提升嘗試、CSRF 拒絕、session 過期，以及「獨立 maintainer 才能合併」規則在真實 session 下仍成立。

**Prompt**

```markdown
Plan (plan mode, no code before approval) milestone M7 — authentication, designed to be
OIDC-ready. OIDC itself is NOT implemented in this milestone.

Design constraints:

1. Separate identity from user: `users` keeps the internal stable id, display name, role,
   disabled flag. Credentials live in `user_identities` (provider, issuer, subject,
   last_login_at). A local password is just one identity type. The unique key for external
   identities is (issuer, subject), never email.
2. All audit data (changes.author_id, changesets, merges) references the internal user id only.
3. Define an auth Provider interface (begin login, handle callback/credentials, return
   normalized identity). Implementations now: local email+password (argon2id) and the
   dev-header provider (development only; startup must fail if enabled in any other
   environment). OIDC is a later implementation of the same interface.
4. Sessions are server-side and independent of the login method; the browser only holds an
   HttpOnly, SameSite cookie. Add CSRF protection for state-changing requests, login rate
   limiting, immediate effect when a user is disabled, and logout.
5. Role source must be replaceable: roles stored on the user now; leave a clean extension
   point for mapping OIDC claims/groups to roles later. A new externally provisioned user
   defaults to the lowest role.
6. Never auto-link accounts by email. Linking an identity to an existing user is an explicit
   maintainer action.
7. Keep a local break-glass maintainer; make "disable local password login for non-admins"
   a config option for the future SSO setup.
8. API (design in contracts/openapi.yaml now): list available providers, start login,
   callback, logout, current user. The web login page renders from the provider list.
9. Maintainer-only user management (create, disable, reset password, change role, view and
   unlink identities).
10. Tests with a fake provider: new user, existing user, disabled user, same email with a
    different subject (must NOT link), role escalation attempts, CSRF rejection, session
    expiry, and the independent-maintainer merge rule under real sessions.

Deliver: a plan (schema, interfaces, endpoints, migrations from the current dev-header
setup), decisions for docs/DECISIONS.md, and open questions. Stop for approval.
```

> 若 M7 已經用簡單的 user + password 做完：改成「遷移任務」——加入 `user_identities` 並搬移現有密碼、抽出 provider 介面、補上不以 email 合併的規則與測試。

### M8 部署與自用

**目標**：能在本機 kind 與一般 Docker 主機上穩定執行，並開始 dogfooding。

範圍：

- API、web 的 Dockerfile（多階段建置、非 root 執行）。
- 完整 compose（db + api + web），與僅開發用的 db compose 並存。
- kind 部署清單（Deployment、Service、Ingress、Postgres、Secret / ConfigMap）；設定一律走環境變數。
- Migration 執行方式（init container 或獨立 job）、健康與就緒檢查。
- 資料庫備份與還原指令（`task backup` / `task restore`），並實測還原。
- CI 建置映像檔；用 Archmap 描述 Archmap 自己的部署架構。

DoD：從乾淨環境用一組指令啟動完整系統；備份後在另一個空環境還原成功。

**Prompt**

```markdown
Plan M8 — deployment and self-hosting (plan mode first).

1. Production Dockerfiles for apps/api and apps/web (multi-stage, non-root, minimal
   images, no secrets baked in).
2. A full-stack compose (db + api + web) alongside the existing db-only compose.
3. Kubernetes manifests for a local kind cluster (Deployment, Service, Ingress, Postgres,
   Secret/ConfigMap), with migrations run as an init container or a Job, plus health and
   readiness probes. All configuration via environment variables; document each one.
4. `task backup` and `task restore` for PostgreSQL, tested by restoring into an empty DB.
5. CI builds the images. Refuse to start in non-development environments if any dev-only
   setting (dev header, default passwords) is enabled.
6. Write the Archmap deployment into Archmap itself as a sample model (dogfooding).
   Deliver the plan and open questions first; stop for approval.
```

### M9 自訂圖示上傳

**目標**：補完 M6 缺的功能；公司內部系統沒有公開圖示，這是累積資產的必要功能。

範圍：

- 上傳 SVG / PNG，嚴格限制大小與類型。
- **SVG 必須淨化**（移除 script、事件屬性、外部參照），避免 XSS；以內容雜湊去重。
- 圖示成為 `TechIcon` 解析順序的最高優先；可把圖示綁定到「技術名稱」，之後同名技術自動套用。
- 儲存位置（資料庫或磁碟 volume）與是否納入版本管理由計畫階段提出建議，記錄於 DECISIONS。
- 備份需涵蓋圖示資料。

DoD：上傳含惡意腳本的 SVG 會被淨化或拒絕（有測試）；自訂圖示在畫布、側邊欄、diff 檢視都能正確顯示，深淺色模式皆可讀。

**Prompt**

```markdown
Plan M9 — custom technology icon upload (plan mode first).

- Upload SVG/PNG with strict size/type limits. SVGs MUST be sanitized server-side
  (strip scripts, event handlers, external references); add tests with malicious SVGs.
- Deduplicate by content hash. Highest priority in TechIcon resolution (custom >
  simple-icons > devicon/logos > lucide). Allow binding an icon to a technology name so
  matching technologies pick it up automatically.
- Propose where to store icons (DB vs volume) and whether they should be versioned through
  changesets; record the decision. Make sure backups (M8) include them.
- Check dark-mode legibility. Update OpenAPI and regenerate clients.
  Deliver the plan and open questions; stop for approval.
```

---

## Phase 2：讓模型更實用

### M10 模型充實（資料庫實例、Owner、環境）（已完成）

> 狀態：已完成（ADR-062..068）。後續擴充：owner 除了具名團隊或個人，也可以指向**使用者**或**群組**（ADR-069）；群組與成員由 maintainer 在 Users 頁管理，不納入版本（ADR-070）。下方第 4 點「與登入的 users 分開」已據此修訂：owner 仍是模型資料，只是可以連結到某個 principal。

**目標**：解決「一個 instance 有很多 database」與「每個資源需要管理人」。

**結構決定**：沿用 C4 階層，不新增層級。

```
node: pg-prod-01
 └─ container: PostgreSQL instance        （資料庫伺服器）
     ├─ component: orders_db               （邏輯資料庫）
     └─ component: billing_db
```

範圍：

1. 新增 `type`（子類型）欄位，與 `kind` 分開：`database-server`、`database`、`schema`、`cache`、`queue`、`proxy` 等；統一目前畫圓柱形的判斷依據；遷移既有資料。
2. Lint「沒有 deployed_on」改用**有效部署**（沿祖先繼承），避免邏輯 DB 被誤報。
3. **按需建模**：只有被應用依賴、或需要獨立管理人的 database 才建立；其餘用 instance 屬性帶過。
4. Owner 以結構化、可版本化的資料處理（走 changeset）：
   - `owners`（`team|person`、聯絡方式、oncall 連結），**與登入的 `users` 分開**，不要綁死（日後可對應 IdP 群組）。
   - `element_owners`（`owner | maintainer | oncall | escalation`）。
   - 可從祖先繼承、可個別覆寫；UI 顯示「有效管理人（來自 …）」。
5. 新欄位：`environment`（prod / staging / dev）、`lifecycle`（active / deprecated / planned）、`criticality`、`runbook_url`、`docs_url`、`last_reviewed_at`。
6. 新 lint：prod 元素無有效管理人 → ERROR；複核過期 → WARN；active 依賴 deprecated → WARN。
7. UI：卡片顯示管理人徽章；依管理人 / 環境 / 生命週期篩選；「**無人管理**」視圖；seed 使用虛構團隊。
8. 這些欄位全部列為 **人工維護欄位**，M12 的同步不得覆蓋（寫進 DECISIONS）。

DoD：繼承、覆寫、lint、diff / merge（含同一 owner 欄位衝突）皆有測試；「無人管理」視圖能列出缺口。

**Prompt**

```markdown
Plan (plan mode, no code before approval) milestone M10 — model enrichment.
Read docs/PROJECT_BRIEF.md, DECISIONS.md and ARCHITECTURE.md first; where they differ
from the code, tell me.

Problem: one database server instance hosts many logical databases, and every
database / container needs a recorded owner.

1. Instance vs databases: keep the C4 hierarchy (server = container, logical database =
   component under it). Add a `type` facet on elements, separate from `kind`
   (e.g. database-server, database, schema, cache, queue, proxy). Unify whatever currently
   drives the cylinder rendering onto this field. Migrate existing data.
2. Lint: change the "no deployed_on" warning to use EFFECTIVE deployment (inherit from
   ancestors), so logical databases are not falsely flagged.
3. Ownership as structured, versioned model data (goes through changesets, diffs and
   history):
   - `owners` (team|person, contact, oncall_url) separate from login `users` (do not couple
     them; a later OIDC group mapping must be possible);
   - `element_owners` (element_id, owner_id, role: owner|maintainer|oncall|escalation);
   - inheritance from ancestors with per-element override; the UI shows the effective
     owner and where it comes from.
4. Add fields: environment (prod|staging|dev), lifecycle (active|deprecated|planned),
   criticality, runbook_url, docs_url, last_reviewed_at.
5. New lint rules: ERROR for prod elements without an effective owner; WARN for stale
   reviews; WARN for active elements depending on deprecated ones.
6. UI: owner badge on cards, filters by owner / environment / lifecycle, and an
   "Unowned" view. Update seed data with fictional teams only (no real contacts).
7. Treat owner / environment / lifecycle as HUMAN-OWNED fields for the future connector
   sync (record this in DECISIONS.md).
8. Update contracts/openapi.yaml, regenerate clients, add tests (inheritance, override,
   lint rules, diff / merge of ownership changes, conflicts on the same owner field).

Deliver: a plan with the schema changes, migration strategy for existing data, API
changes, and open questions. Stop for my approval before coding.
```

### M10.1 聚焦式 Canvas（drill-down）（已完成）

**目標**：模型變大後，單一畫布難以閱讀；改用 C4 / IcePanel 式的分層瀏覽。

範圍（ADR-071）：

- 雙擊有子元素的卡片即進入聚焦，只顯示該元素的子樹，以及與它有關係的外部元素（標示為 External、不可展開）。外部元素取「不包含聚焦元素的最高祖先」，與伺服器端 container view 的規則一致。
- 聚焦狀態放在網址（`?focus=`），可分享、支援上一頁。`F` 聚焦選取的元素，`Esc` 回上一層，麵包屑在畫布工具島內。
- 聚焦時的展開狀態與拖曳只存在記憶體，不寫入 view 的版面。可以「存成 view」，建立 container 或 component view。
- 大型模型：ELK 改在 web worker 執行、React Flow 只渲染可見元素、連線聚合不再逐條走祖先鏈。元素超過 300 個時，「Expand all」改為一次展開一層。

DoD：main、草稿、版本頁都可聚焦，diff 標記正確；聚焦不影響已存的 view 版面（有測試）。

### M10.2 可設定審核政策（直接編輯）（已完成）

**目標**：個人或小團隊使用時，不必每次修改都走草稿與審核，但保留歷史、還原與治理能力。

範圍（ADR-072）：

- 全域設定 `review_policy`：`required`（預設，行為不變）或 `direct`。只有 maintainer 能修改，設定不納入版本。
- `direct` 模式下，編輯者在 main 上直接編輯，每次儲存是一個自動合併的 changeset（一個版本）。沿用同一套驗證、diff、版本與快照，沒有第二條寫入路徑。
- 治理仍然有效：
  - 新增 lint 錯誤的修改會保留為 draft 讓人修正。
  - 動到受保護元素的修改會送審，由另一位 maintainer 審核。
- 復原：對自己最近一次直接編輯產生反向版本。若之後有人改過相同欄位，則拒絕復原。
- 版本頁把同一人連續的直接編輯收合為一組；main 每 15 秒與取得焦點時檢查新版本。

DoD：權限、政策切換、lint / 受保護元素的分流、並行直接編輯、復原與衝突皆有測試。

**後續（未排入）**：用 SSE 取代輪詢、presence（誰正在看）、同一份草稿內的即時共編。

### M11 YAML / JSON 匯入匯出

**目標**：能批次進出資料、可進 Git，也為連接器鋪路。

範圍：

- 有版本號的檔案格式（`schemaVersion`），包含 M10 新增的所有欄位（type、owner、environment 等）。發布 JSON Schema 供編輯器驗證。
- 匯出：某個 model 版本、某個 view 或某個子樹。
- **匯入一律產生 draft**，不直接寫入 main（即使審核政策是 `direct`）；先 dry-run 顯示將新增 / 修改 / 刪除什麼，並跑 lint。
- 以穩定的外部鍵比對（不是每次產生新 UUID），重複匯入具冪等性。
- 大小與筆數限制、清楚的錯誤訊息（含行號）。
- Round-trip 測試：匯出 → 匯入 → 再匯出，內容一致。

DoD：把 seed 模型匯出再匯入到空資料庫，結果與原模型等價；重複匯入同一份檔案不產生變更。

**Prompt**

```markdown
Plan M11 — YAML/JSON import and export (plan mode first).

- A versioned file format (schemaVersion) covering ALL model fields including type,
  owners, environment, lifecycle, criticality and links. Publish a JSON Schema.
- Export: a model version, a view, or a subtree.
- Import ALWAYS creates a DRAFT changeset (never touches main). Provide a dry-run that
  reports adds / updates / deletes, runs lint, and reports errors with line numbers.
- Match entities by stable external keys so re-importing the same file is idempotent;
  do not mint new UUIDs on every run.
- Enforce size/count limits. Round-trip test: export -> import into an empty DB -> export
  again must be equivalent.
  Deliver the plan and open questions; stop for approval.
```

---

## Phase 3：讓資料自動進來

### M12 連接器框架 + OpenEverest

**目標**：建立可重複使用的「探索 → draft」管線，並以 OpenEverest 作為第一個連接器。

OpenEverest 事實（需再驗證）：核心 CRD 為 `DatabaseCluster`，支援 PostgreSQL、MySQL（PXC）、MongoDB（PSMDB）；同時有 REST API（官方建議多數操作走 UI / API）。**API 文件頁是 JS 渲染，具體 endpoint、欄位與認證請直接讀其 GitHub 上的 OpenAPI spec，不要沿用舊版 Percona Everest 的記憶。**

範圍：

1. `external_refs`（element_id、source、external_id、last_synced_at、content_hash），讓同步具冪等性並可偵測漂移。
2. **欄位擁有者規則**：探索維護 replicas / version / storage 等；人工維護 owner / environment / lifecycle / description / tags / 手動關係。同步永不覆蓋人工欄位。
3. 連接器介面（Go）：`Discover(ctx)` → 提案；`Status(ctx, bindings)` → 執行期狀態（M13 使用）。
4. 探索結果 → draft。重複同步與目前 main 比較，產生「新增 / 變更 / 疑似已移除」提案；**移除永不自動執行**。
5. 對應：K8s cluster / namespace → 巢狀 `node`；`DatabaseCluster` → `container`（type = database-server，technology = postgresql / mysql / mongodb）；proxy → `component`；`deployed_on` → namespace node。
6. 唯讀，使用最小權限帳號；憑證加密儲存、不寫入 log；連接器設定僅 maintainer 可管理。
7. 測試：以錄製回應的 mock 伺服器跑 CI；另附在 kind 安裝 OpenEverest 的手動驗證步驟。
8. 邏輯 database（instance 內的各個 DB）是否能由 API 取得需確認；若不能，維持手動建模，或日後另做資料庫探索連接器。

DoD：對 mock 與（可得時）真實 OpenEverest 同步兩次，第二次無變更；修改外部資料後產生正確的變更與「疑似已移除」提案；人工欄位不被覆蓋（有測試）。

**Prompt**

```markdown
Plan the connector framework and the OpenEverest connector. Use plan mode and do NOT
write code before I approve the plan.

Context: OpenEverest (formerly Percona Everest) manages databases on Kubernetes through
the DatabaseCluster CRD and a REST API (docs: https://openeverest.io/docs/api/, which is
JS-rendered; find the actual OpenAPI spec in the openeverest GitHub repo and read it).
Verify against the current spec: endpoints, auth (session / token), DatabaseCluster
fields and status values, and whether logical databases inside an instance can be listed.
Do not assume anything from older Percona Everest versions.

Requirements:

1. Read-only. Prefer the REST API with a read-only account; note whether CRD access via
   client-go would be needed for anything the API lacks.
2. Add `external_refs` (element_id, source, external_id, last_synced_at, content_hash)
   so repeated syncs are idempotent, plus field ownership rules (discovery-owned vs
   human-owned fields; sync never overwrites human-owned ones: owner, environment,
   lifecycle, criticality, description, tags, manual relations).
3. Connector interface in Go: Discover(ctx) -> proposed elements/relations,
   Status(ctx, bindings) -> runtime status. OpenEverest is the first implementation.
4. Discovery output becomes a DRAFT changeset (never writes to main). Re-sync produces
   a new draft with add / change / "possibly removed" proposals; removals are never automatic.
5. Mapping: cluster/namespace -> nested `node`; DatabaseCluster -> `container` with
   type database-server, technology (postgresql / mysql / mongodb) and properties
   (version, replicas, storage, expose type); proxy -> component; `deployed_on` relation
   to the namespace node.
6. Runtime status (phase / ready) goes through `bindings`, NOT through changesets or
   versions (implemented in M13; design the interface now).
7. Credentials stored encrypted, never logged; connector configuration is maintainer-only.
8. Testing: mock server with recorded responses for CI; a documented manual test against
   OpenEverest installed in kind.

Deliver: docs/DECISIONS.md entries, a milestone breakdown (refs + connector interface +
import-to-draft first, then the OpenEverest Discover), and open questions.
```

---

## Phase 4：讓圖「活起來」

### M13 即時狀態（Prometheus 與 OpenEverest）

**目標**：在節點上顯示執行期狀態，且不污染模型與版本。

範圍：

- 啟用預留的 `bindings`：元素綁定狀態來源（OpenEverest phase、Prometheus 查詢、HTTP health URL）。
- **狀態是唯讀的執行期資料**：不進 changeset、不產生版本，與 Layout 分開處理。
- 後端統一輪詢與快取（避免每個元素各打一次查詢）；批次查詢、逾時、錯誤隔離（單一來源失敗不影響其他）。
- 狀態有「過期」概念：資料超過 N 秒未更新要顯示為未知，而不是顯示舊的綠燈。
- 前端以輪詢或 SSE 更新，節點顯示狀態徽章與顏色；收合的父節點顯示子節點的最差狀態。
- Binding 設定本身屬於模型（可 review），但憑證與查詢結果不是。
- 嚴格限制查詢語句的來源與權限，避免變成任意查詢代理（SSRF / 資料外洩）。

DoD：關閉某個來源時，相關節點顯示「未知 / 過期」且其他節點不受影響；父節點正確彙整最差狀態。

**Prompt**

```markdown
Plan M13 — live status (plan mode first).

- Implement `bindings` (OpenEverest phase, Prometheus query, HTTP health URL). Binding
  definitions are model data (reviewable); status results and credentials are NOT.
- Runtime status never enters changesets or versions. Backend polls centrally with caching,
  batching, timeouts and per-source error isolation. Status has a staleness TTL: stale
  data renders as "unknown", never as a green light.
- Frontend updates via polling or SSE; nodes show a status badge; collapsed parents show
  the worst child status. Respect prefers-reduced-motion.
- Security: restrict what queries/URLs may be configured and who may configure them
  (maintainers only); defend against SSRF; do not allow arbitrary query proxying.
- Tests: source down, slow source, stale data, aggregation to the worst status.
  Deliver the plan and open questions; stop for approval.
```

### M14 API endpoint 目錄

**目標**：補上原始需求中的 API endpoints。

範圍：

- Endpoint 掛在 container 之下（HTTP 方法 + 路徑 + 說明 + 版本），**預設不畫成節點**（數量可能很大），在側邊欄與詳細面板中瀏覽。
- 匯入 OpenAPI 檔，產生 draft；與既有 endpoint 以外部鍵比對。
- 關係可指向 endpoint（例如「某前端呼叫 billing-api 的 `POST /invoices`」）；在畫布上聚合顯示為 container 之間的關係，展開才看到細節。
- Lint：未被任何人使用的 endpoint（WARN）、已標示 deprecated 卻仍被使用（WARN）。
- 是否把 endpoint 做成新的 element kind 或獨立資料表，由計畫階段提出建議並記錄。

DoD：匯入一份真實 OpenAPI 後，endpoint 可搜尋；consumer 關係在收合時聚合、展開時可見。

**Prompt**

```markdown
Plan M14 — API endpoint catalog (plan mode first).

- Endpoints belong to a container (method, path, summary, version, deprecated flag). Do not
  render them as canvas nodes by default; browse them in the sidebar / detail panel.
- Import an OpenAPI file into a DRAFT; match existing endpoints by stable external keys.
- Relations may target an endpoint; on the canvas they aggregate to container-to-container
  edges and reveal details on expand.
- Lint: unused endpoints (WARN), deprecated but still used (WARN).
- Propose whether endpoints are a new element kind or a separate table, with trade-offs,
  and record the decision. Update OpenAPI and regenerate clients.
  Deliver the plan and open questions; stop for approval.
```

---

## Phase 5：企業整合

### M15 OIDC

**目標**：以 M7 的 provider 介面加入 OIDC，不改動 schema 與授權邏輯。

範圍：

- Provider 設定（issuer、client id / secret、scopes、redirect URI），可設定多個 provider。
- 授權碼流程搭配 **PKCE**、驗證 `state` 與 `nonce`，並驗證 token 的簽章、issuer、audience、過期時間。
- 首次登入建立帳號（JIT provisioning）預設為最低角色；**群組 / claims → 角色**的對應規則表（maintainer 可設定）。
- 不以 email 自動合併；連結既有帳號為明確動作。
- 登出：清除本機 session；是否一併登出 IdP 為選項。
- 可選：把 IdP 群組同步成 Archmap 群組（ADR-070）的成員，讓群組 owner 自動跟著 IdP 更新。
- 測試環境用 compose 起本機 OIDC 提供者（Keycloak、Dex 或輕量 mock，選擇時先查證現況），不依賴外部服務。
- 啟用後既有本機帳號的處理（保留 break-glass、其餘依設定停用密碼登入）。

DoD：以本機 OIDC 完成登入、群組對應角色、停用使用者即時失效、同 email 不同 subject 不被合併（有測試）。

**Prompt**

```markdown
Plan M15 — OIDC login as a new implementation of the M7 Provider interface (plan mode
first; no schema or authorization changes beyond configuration tables).

- Provider config (issuer, client id/secret, scopes), multiple providers allowed.
- Authorization code flow with PKCE; validate state, nonce, signature, issuer, audience,
  expiry. Verify current Go OIDC library choices from official docs.
- JIT provisioning at the lowest role; maintainer-configurable group/claims -> role rules.
- Never auto-link by email; explicit linking only. Keep the break-glass local maintainer;
  support disabling local passwords for non-admins.
- Local OIDC test environment in compose (evaluate Keycloak / Dex / a mock; check current
  status), no dependency on external services.
- Optional: map IdP groups to `owners` (see M10).
  Deliver the plan and open questions; stop for approval.
```

---

## Backlog（之後再說）

- K8s / Docker 探索連接器（使用 M12 的框架）
- Changeset 留言、通知、Webhook
- 匯出 PNG / SVG / Mermaid
- SSE 即時通知（取代 M10.2 的輪詢）、Presence（誰正在看 / 編輯）與草稿內即時共編（CRDT）
- 針對邏輯資料庫的探索連接器（例如讀取 PostgreSQL catalog）
- IdP 群組與 Archmap 群組成員的自動同步（見 M15）
- Prometheus 以外的狀態來源

## 開放問題（會影響順序）

1. **使用情境**：先給同事試用，還是先自己整理現有環境？前者優先 M7、M8；後者可把 M10、M11 提前。
2. **OpenEverest 環境**：是已經在運作的環境，還是要在 kind 新裝來測試？前者能直接用真實回應設計對應；後者要先安排安裝與準備資料。
3. **應用程式依賴的粒度**：多數依賴只到 instance 就夠，還是需要精確到邏輯 database？若只到 instance，M10 的邏輯 DB 建模可縮小範圍。
4. ~~**M7 現況**~~：已解決，M7 以 session 登入完成（ADR-053..061）。

## 每個里程碑的標準流程

1. 開新 session（讓 `AGENTS.md` / `CLAUDE.md` 自動帶入背景）。
2. 貼上該里程碑的 Prompt，先看計畫與未決問題並確認。
3. 完成後檢查：測試、diff、新增的 DECISIONS 條目、文件是否同步。
4. 通過後再進入下一個里程碑。
