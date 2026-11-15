# Detailed logical ERD

Status: design proposal. Each diagram shows primary keys and foreign keys only, because the data dictionary has every column. Entities that appear in a diagram only as the target of a reference are repeated from their home domain. A dashed line is not used: Mermaid draws every relationship the same way, so optional relationships are shown with `|o` on the parent side (the child may have no parent) and mandatory ones with `||`. `o{` means zero or many children. `o|` means zero or one child (a one-to-one link).

Ownership versus operational relationships. **Ownership** is the program: every table that has `program_id` belongs to exactly one program through a composite foreign key (DB-09). **Operational** relationships record what happened to something: an attempt belongs to a job, a job consumed an approval use, an observation came from an attempt. The separate diagrams below group both kinds by domain. The cross-domain paths that matter are in `erd-high-level.md`.


## Identity and policy

```mermaid
erDiagram
    users {
        uuid id PK
    }
    user_credentials {
        uuid id PK
        uuid user_id FK
    }
    recovery_codes {
        uuid id PK
        uuid user_id FK
    }
    bootstrap_secrets {
        uuid id PK
        uuid consumed_user_id FK
    }
    sessions {
        uuid id PK
        uuid user_id FK
    }
    reauth_grants {
        uuid id PK
        uuid user_id FK
        uuid session_id FK
    }
    login_attempts {
        uuid id PK
        uuid user_id FK
    }
    policy_settings {
        uuid id PK
        uuid changed_by FK
        uuid reauth_grant_id FK
    }
    recovery_state {
        boolean singleton PK
    }
    users ||--o{ user_credentials : "user_id"
    users ||--o{ recovery_codes : "user_id"
    users |o--o{ bootstrap_secrets : "consumed_user_id"
    users ||--o{ sessions : "user_id"
    users ||--o{ reauth_grants : "user_id"
    sessions ||--o{ reauth_grants : "session_id"
    users |o--o{ login_attempts : "user_id"
    users ||--o{ policy_settings : "changed_by"
    reauth_grants |o--o{ policy_settings : "reauth_grant_id"
```

## Programs, scope and target authorization

```mermaid
erDiagram
    programs {
        uuid id PK
        uuid owner_user_id FK
    }
    program_rules_versions {
        uuid id PK
        uuid program_id FK
        uuid recorded_by FK
    }
    scope_versions {
        uuid id PK
        uuid program_id FK
        uuid rules_version_id FK
        uuid supersedes_version_id FK
        uuid created_by FK
        uuid reauth_grant_id FK
    }
    scope_rules {
        uuid id PK
        uuid program_id FK
        uuid scope_version_id FK
    }
    refused_exclusions {
        uuid id PK
        uuid program_id FK
        uuid scope_version_id FK
        uuid acknowledged_by FK
        uuid replaced_by_rule_id FK
        uuid reauth_grant_id FK
    }
    targets {
        uuid id PK
        uuid program_id FK
        uuid promoted_from_asset_id FK
        uuid created_by FK
    }
    lab_registrations {
        uuid id PK
        uuid program_id FK
        uuid assessment_id FK
        uuid target_id FK
        uuid scope_version_id FK
        uuid registered_by FK
        uuid reauth_grant_id FK
        uuid revoked_by FK
        uuid supersedes_lab_id FK
    }
    lab_registration_ranges {
        uuid id PK
        uuid lab_registration_id FK
    }
    target_validations {
        uuid id PK
        uuid program_id FK
        uuid assessment_id FK
        uuid scope_version_id FK
        uuid rules_version_id FK
        uuid attested_by FK
    }
    target_validation_targets {
        uuid id PK
        uuid validation_id FK
        uuid target_id FK
        uuid matched_scope_rule_id FK
        uuid lab_registration_id FK
    }
    assessments {
        uuid id PK
        uuid program_id FK
        uuid current_validation_id FK
        uuid created_by FK
    }
    programs ||--o{ program_rules_versions : "program_id"
    programs ||--o{ scope_versions : "program_id"
    program_rules_versions |o--o{ scope_versions : "rules_version_id"
    scope_versions |o--o{ scope_versions : "supersedes_version_id"
    programs ||--o{ scope_rules : "program_id"
    scope_versions ||--o{ scope_rules : "scope_version_id"
    programs ||--o{ refused_exclusions : "program_id"
    scope_versions ||--o{ refused_exclusions : "scope_version_id"
    scope_rules |o--o{ refused_exclusions : "replaced_by_rule_id"
    programs ||--o{ targets : "program_id"
    programs ||--o{ lab_registrations : "program_id"
    assessments ||--o{ lab_registrations : "assessment_id"
    targets ||--o{ lab_registrations : "target_id"
    scope_versions ||--o{ lab_registrations : "scope_version_id"
    lab_registrations |o--o{ lab_registrations : "supersedes_lab_id"
    lab_registrations ||--o{ lab_registration_ranges : "lab_registration_id"
    programs ||--o{ target_validations : "program_id"
    assessments ||--o{ target_validations : "assessment_id"
    scope_versions ||--o{ target_validations : "scope_version_id"
    program_rules_versions ||--o{ target_validations : "rules_version_id"
    target_validations ||--o{ target_validation_targets : "validation_id"
    targets ||--o{ target_validation_targets : "target_id"
    scope_rules |o--o{ target_validation_targets : "matched_scope_rule_id"
    lab_registrations |o--o{ target_validation_targets : "lab_registration_id"
    programs ||--o{ assessments : "program_id"
    target_validations |o--o{ assessments : "current_validation_id"
```

## Assessment, authorization and execution

```mermaid
erDiagram
    assessments {
        uuid id PK
        uuid program_id FK
        uuid current_validation_id FK
        uuid created_by FK
    }
    assessment_targets {
        uuid assessment_id PK, FK
        uuid target_id PK, FK
        uuid program_id FK
        uuid added_by FK
    }
    assessment_transitions {
        uuid id PK
        uuid assessment_id FK
        uuid actor_user_id FK
    }
    plan_items {
        uuid id PK
        uuid program_id FK
        uuid assessment_id FK
        uuid target_id FK
        uuid operation_id FK
        uuid ai_job_id FK
        uuid decided_by FK
    }
    operations {
        uuid id PK
        uuid program_id FK
        uuid assessment_id FK
    }
    policy_decisions {
        uuid id PK
        uuid program_id FK
        uuid assessment_id FK
        uuid operation_id FK
        uuid job_id FK
        uuid scope_version_id FK
        uuid validation_id FK
        uuid approval_id FK
        uuid lab_registration_id FK
    }
    approvals {
        uuid id PK
        uuid program_id FK
        uuid assessment_id FK
        uuid operation_id FK
        uuid requested_by_user_id FK
        uuid decided_by FK
        uuid validation_id FK
        uuid scope_version_id FK
        uuid reauth_grant_id FK
        uuid revoked_by FK
    }
    approval_events {
        uuid id PK
        uuid approval_id FK
        uuid actor_user_id FK
        uuid job_id FK
    }
    approval_uses {
        uuid id PK
        uuid approval_id FK
        uuid job_id FK
    }
    jobs {
        uuid id PK
        uuid program_id FK
        uuid assessment_id FK
        uuid operation_id FK
        uuid plan_item_id FK
        uuid approval_id FK
        uuid approval_use_id FK
        uuid scope_version_id FK
        uuid validation_id FK
        uuid created_by FK
    }
    job_attempts {
        uuid id PK
        uuid job_id FK
        uuid approval_use_id FK
        uuid decision_id FK
    }
    job_transitions {
        uuid id PK
        uuid job_id FK
        uuid actor_user_id FK
    }
    targets {
        uuid id PK
        uuid program_id FK
        uuid promoted_from_asset_id FK
        uuid created_by FK
    }
    target_validations {
        uuid id PK
        uuid program_id FK
        uuid assessment_id FK
        uuid scope_version_id FK
        uuid rules_version_id FK
        uuid attested_by FK
    }
    scope_versions {
        uuid id PK
        uuid program_id FK
        uuid rules_version_id FK
        uuid supersedes_version_id FK
        uuid created_by FK
        uuid reauth_grant_id FK
    }
    target_validations |o--o{ assessments : "current_validation_id"
    assessments ||--o{ assessment_targets : "assessment_id"
    targets ||--o{ assessment_targets : "target_id"
    assessments ||--o{ assessment_transitions : "assessment_id"
    assessments ||--o{ plan_items : "assessment_id"
    targets ||--o{ plan_items : "target_id"
    operations ||--o{ plan_items : "operation_id"
    jobs |o--o{ plan_items : "ai_job_id"
    assessments ||--o{ operations : "assessment_id"
    assessments ||--o{ policy_decisions : "assessment_id"
    operations |o--o{ policy_decisions : "operation_id"
    jobs |o--o{ policy_decisions : "job_id"
    scope_versions ||--o{ policy_decisions : "scope_version_id"
    target_validations |o--o{ policy_decisions : "validation_id"
    approvals |o--o{ policy_decisions : "approval_id"
    assessments ||--o{ approvals : "assessment_id"
    operations ||--o{ approvals : "operation_id"
    target_validations ||--o{ approvals : "validation_id"
    scope_versions ||--o{ approvals : "scope_version_id"
    approvals ||--o{ approval_events : "approval_id"
    jobs |o--o{ approval_events : "job_id"
    approvals ||--o{ approval_uses : "approval_id"
    jobs ||--o{ approval_uses : "job_id"
    assessments ||--o{ jobs : "assessment_id"
    operations |o--o{ jobs : "operation_id"
    plan_items |o--o{ jobs : "plan_item_id"
    approvals |o--o{ jobs : "approval_id"
    approval_uses |o--o{ jobs : "approval_use_id"
    scope_versions ||--o{ jobs : "scope_version_id"
    target_validations |o--o{ jobs : "validation_id"
    jobs ||--o{ job_attempts : "job_id"
    approval_uses |o--o{ job_attempts : "approval_use_id"
    policy_decisions ||--o{ job_attempts : "decision_id"
    jobs ||--o{ job_transitions : "job_id"
    assessments ||--o{ target_validations : "assessment_id"
    scope_versions ||--o{ target_validations : "scope_version_id"
    scope_versions |o--o{ scope_versions : "supersedes_version_id"
```

## Asset intelligence

```mermaid
erDiagram
    assets {
        uuid id PK
        uuid program_id FK
        uuid scope_version_id FK
    }
    services {
        uuid id PK
        uuid program_id FK
        uuid asset_id FK
    }
    endpoints {
        uuid id PK
        uuid program_id FK
        uuid service_id FK
    }
    endpoint_parameters {
        uuid id PK
        uuid program_id FK
        uuid endpoint_id FK
    }
    observations {
        uuid id PK
        uuid program_id FK
        uuid asset_id FK
        uuid service_id FK
        uuid endpoint_id FK
        uuid relationship_id FK
        uuid job_id FK
        uuid attempt_id FK
        uuid evidence_id FK
        uuid scope_version_id FK
    }
    asset_relationships {
        uuid id PK
        uuid program_id FK
        uuid from_asset_id FK
        uuid to_asset_id FK
    }
    targets {
        uuid id PK
        uuid program_id FK
        uuid promoted_from_asset_id FK
        uuid created_by FK
    }
    jobs {
        uuid id PK
        uuid program_id FK
        uuid assessment_id FK
        uuid operation_id FK
        uuid plan_item_id FK
        uuid approval_id FK
        uuid approval_use_id FK
        uuid scope_version_id FK
        uuid validation_id FK
        uuid created_by FK
    }
    job_attempts {
        uuid id PK
        uuid job_id FK
        uuid approval_use_id FK
        uuid decision_id FK
    }
    evidence_items {
        uuid id PK
        uuid program_id FK
        uuid assessment_id FK
        uuid job_id FK
        uuid attempt_id FK
        uuid target_id FK
        uuid decision_id FK
    }
    scope_versions {
        uuid id PK
        uuid program_id FK
        uuid rules_version_id FK
        uuid supersedes_version_id FK
        uuid created_by FK
        uuid reauth_grant_id FK
    }
    scope_versions |o--o{ assets : "scope_version_id"
    assets ||--o{ services : "asset_id"
    services ||--o{ endpoints : "service_id"
    endpoints ||--o{ endpoint_parameters : "endpoint_id"
    assets |o--o{ observations : "asset_id"
    services |o--o{ observations : "service_id"
    endpoints |o--o{ observations : "endpoint_id"
    asset_relationships |o--o{ observations : "relationship_id"
    jobs |o--o{ observations : "job_id"
    job_attempts |o--o{ observations : "attempt_id"
    evidence_items |o--o{ observations : "evidence_id"
    scope_versions |o--o{ observations : "scope_version_id"
    assets ||--o{ asset_relationships : "from_asset_id"
    assets ||--o{ asset_relationships : "to_asset_id"
    assets |o--o{ targets : "promoted_from_asset_id"
    scope_versions ||--o{ jobs : "scope_version_id"
    jobs ||--o{ job_attempts : "job_id"
    jobs |o--o{ evidence_items : "job_id"
    job_attempts |o--o{ evidence_items : "attempt_id"
    targets |o--o{ evidence_items : "target_id"
    scope_versions |o--o{ scope_versions : "supersedes_version_id"
```

## Evidence, findings and reports

```mermaid
erDiagram
    evidence_items {
        uuid id PK
        uuid program_id FK
        uuid assessment_id FK
        uuid job_id FK
        uuid attempt_id FK
        uuid target_id FK
        uuid decision_id FK
    }
    evidence_integrity_checks {
        uuid id PK
        uuid evidence_id FK
        uuid actor_user_id FK
    }
    evidence_status {
        uuid evidence_id PK, FK
        uuid last_check_id FK
        uuid purged_by FK
    }
    findings {
        uuid id PK, FK
        uuid program_id FK
        uuid assessment_id FK
        uuid asset_id FK
        uuid endpoint_id FK
        uuid verification_id FK
        uuid created_by_user_id FK
    }
    finding_transitions {
        uuid id PK
        uuid finding_id FK
        uuid actor_user_id FK
        uuid validation_id FK
    }
    finding_evidence {
        uuid finding_id PK, FK
        uuid evidence_id PK, FK
        uuid program_id FK
        uuid validation_id FK
        uuid linked_by_user_id FK
    }
    validation_records {
        uuid id PK
        uuid finding_id FK
        uuid replay_job_id FK
        uuid recorded_by_user_id FK
    }
    finding_verifications {
        uuid id PK
        uuid finding_id FK
        uuid verified_by FK
        uuid validation_id FK
        uuid reauth_grant_id FK
        uuid superseded_by_transition_id FK
    }
    finding_verification_evidence {
        uuid verification_id PK, FK
        uuid evidence_id PK, FK
        uuid integrity_check_id FK
    }
    reports {
        uuid id PK
        uuid program_id FK
        uuid assessment_id FK
        uuid created_by FK
        uuid finalized_by FK
    }
    report_findings {
        uuid report_id PK, FK
        uuid finding_id PK, FK
        uuid program_id FK
        uuid verification_id FK
        uuid included_by FK
    }
    assets {
        uuid id PK
        uuid program_id FK
        uuid scope_version_id FK
    }
    endpoints {
        uuid id PK
        uuid program_id FK
        uuid service_id FK
    }
    jobs {
        uuid id PK
        uuid program_id FK
        uuid assessment_id FK
        uuid operation_id FK
        uuid plan_item_id FK
        uuid approval_id FK
        uuid approval_use_id FK
        uuid scope_version_id FK
        uuid validation_id FK
        uuid created_by FK
    }
    job_attempts {
        uuid id PK
        uuid job_id FK
        uuid approval_use_id FK
        uuid decision_id FK
    }
    targets {
        uuid id PK
        uuid program_id FK
        uuid promoted_from_asset_id FK
        uuid created_by FK
    }
    policy_decisions {
        uuid id PK
        uuid program_id FK
        uuid assessment_id FK
        uuid operation_id FK
        uuid job_id FK
        uuid scope_version_id FK
        uuid validation_id FK
        uuid approval_id FK
        uuid lab_registration_id FK
    }
    jobs |o--o{ evidence_items : "job_id"
    job_attempts |o--o{ evidence_items : "attempt_id"
    targets |o--o{ evidence_items : "target_id"
    policy_decisions |o--o{ evidence_items : "decision_id"
    evidence_items ||--o{ evidence_integrity_checks : "evidence_id"
    evidence_items ||--o| evidence_status : "evidence_id"
    evidence_integrity_checks |o--o{ evidence_status : "last_check_id"
    assets |o--o{ findings : "asset_id"
    endpoints |o--o{ findings : "endpoint_id"
    finding_verifications |o--o{ findings : "verification_id"
    findings ||--o{ finding_transitions : "finding_id"
    validation_records |o--o{ finding_transitions : "validation_id"
    findings ||--o{ finding_evidence : "finding_id"
    evidence_items ||--o{ finding_evidence : "evidence_id"
    validation_records |o--o{ finding_evidence : "validation_id"
    findings ||--o{ validation_records : "finding_id"
    jobs |o--o{ validation_records : "replay_job_id"
    findings ||--o{ finding_verifications : "finding_id"
    validation_records ||--o{ finding_verifications : "validation_id"
    finding_transitions |o--o{ finding_verifications : "superseded_by_transition_id"
    finding_verifications ||--o{ finding_verification_evidence : "verification_id"
    evidence_items ||--o{ finding_verification_evidence : "evidence_id"
    evidence_integrity_checks ||--o{ finding_verification_evidence : "integrity_check_id"
    reports ||--o{ report_findings : "report_id"
    findings ||--o{ report_findings : "finding_id"
    finding_verifications |o--o{ report_findings : "verification_id"
    jobs ||--o{ job_attempts : "job_id"
    policy_decisions ||--o{ job_attempts : "decision_id"
    assets |o--o{ targets : "promoted_from_asset_id"
    jobs |o--o{ policy_decisions : "job_id"
```

## AI and integrations

```mermaid
erDiagram
    credential_refs {
        uuid id PK
        uuid created_by FK
    }
    ai_providers {
        uuid id PK
        uuid credential_ref_id FK
        uuid created_by FK
    }
    program_provider_enablements {
        uuid program_id FK
        uuid provider_id PK, FK
        uuid enabled_by FK
        uuid reauth_grant_id FK
    }
    ai_transmissions {
        uuid id PK
        uuid provider_id FK
        uuid program_id FK
        uuid job_id FK
        uuid audit_event_id FK
    }
    ai_outputs {
        uuid id PK
        uuid program_id FK
        uuid assessment_id FK
        uuid job_id FK
    }
    tool_configs {
        uuid id PK
        uuid registered_by FK
        uuid reauth_grant_id FK
    }
    passive_source_enablements {
        uuid id PK
        uuid program_id FK
        uuid enabled_by FK
        uuid reauth_grant_id FK
    }
    integration_checks {
        uuid id PK
    }
    idempotency_keys {
        uuid id PK
        uuid user_id FK
    }
    programs {
        uuid id PK
        uuid owner_user_id FK
    }
    jobs {
        uuid id PK
        uuid program_id FK
        uuid assessment_id FK
        uuid operation_id FK
        uuid plan_item_id FK
        uuid approval_id FK
        uuid approval_use_id FK
        uuid scope_version_id FK
        uuid validation_id FK
        uuid created_by FK
    }
    audit_events {
        uuid id PK
        uuid actor_user_id FK
        uuid program_id FK
        uuid assessment_id FK
        uuid job_id FK
        uuid scope_version_id FK
        uuid decision_id FK
        uuid approval_id FK
    }
    reauth_grants {
        uuid id PK
        uuid user_id FK
        uuid session_id FK
    }
    credential_refs |o--o{ ai_providers : "credential_ref_id"
    programs ||--o{ program_provider_enablements : "program_id"
    ai_providers ||--o| program_provider_enablements : "provider_id"
    reauth_grants ||--o{ program_provider_enablements : "reauth_grant_id"
    ai_providers ||--o{ ai_transmissions : "provider_id"
    programs ||--o{ ai_transmissions : "program_id"
    jobs |o--o{ ai_transmissions : "job_id"
    audit_events ||--o{ ai_transmissions : "audit_event_id"
    programs ||--o{ ai_outputs : "program_id"
    jobs ||--o{ ai_outputs : "job_id"
    reauth_grants ||--o{ tool_configs : "reauth_grant_id"
    programs ||--o{ passive_source_enablements : "program_id"
    reauth_grants ||--o{ passive_source_enablements : "reauth_grant_id"
    programs ||--o{ jobs : "program_id"
    programs |o--o{ audit_events : "program_id"
    jobs |o--o{ audit_events : "job_id"
```

## Audit and cross-cutting references

```mermaid
erDiagram
    audit_events {
        uuid id PK
        uuid actor_user_id FK
        uuid program_id FK
        uuid assessment_id FK
        uuid job_id FK
        uuid scope_version_id FK
        uuid decision_id FK
        uuid approval_id FK
    }
    system_alerts {
        uuid id PK
        uuid cleared_by FK
    }
    users {
        uuid id PK
    }
    programs {
        uuid id PK
        uuid owner_user_id FK
    }
    assessments {
        uuid id PK
        uuid program_id FK
        uuid current_validation_id FK
        uuid created_by FK
    }
    jobs {
        uuid id PK
        uuid program_id FK
        uuid assessment_id FK
        uuid operation_id FK
        uuid plan_item_id FK
        uuid approval_id FK
        uuid approval_use_id FK
        uuid scope_version_id FK
        uuid validation_id FK
        uuid created_by FK
    }
    scope_versions {
        uuid id PK
        uuid program_id FK
        uuid rules_version_id FK
        uuid supersedes_version_id FK
        uuid created_by FK
        uuid reauth_grant_id FK
    }
    policy_decisions {
        uuid id PK
        uuid program_id FK
        uuid assessment_id FK
        uuid operation_id FK
        uuid job_id FK
        uuid scope_version_id FK
        uuid validation_id FK
        uuid approval_id FK
        uuid lab_registration_id FK
    }
    approvals {
        uuid id PK
        uuid program_id FK
        uuid assessment_id FK
        uuid operation_id FK
        uuid requested_by_user_id FK
        uuid decided_by FK
        uuid validation_id FK
        uuid scope_version_id FK
        uuid reauth_grant_id FK
        uuid revoked_by FK
    }
    users |o--o{ audit_events : "actor_user_id"
    programs |o--o{ audit_events : "program_id"
    assessments |o--o{ audit_events : "assessment_id"
    jobs |o--o{ audit_events : "job_id"
    scope_versions |o--o{ audit_events : "scope_version_id"
    policy_decisions |o--o{ audit_events : "decision_id"
    approvals |o--o{ audit_events : "approval_id"
    users |o--o{ system_alerts : "cleared_by"
    users ||--o{ programs : "owner_user_id"
    programs ||--o{ assessments : "program_id"
    users ||--o{ assessments : "created_by"
    programs ||--o{ jobs : "program_id"
    assessments ||--o{ jobs : "assessment_id"
    approvals |o--o{ jobs : "approval_id"
    scope_versions ||--o{ jobs : "scope_version_id"
    users |o--o{ jobs : "created_by"
    programs ||--o{ scope_versions : "program_id"
    scope_versions |o--o{ scope_versions : "supersedes_version_id"
    users ||--o{ scope_versions : "created_by"
    programs ||--o{ policy_decisions : "program_id"
    assessments ||--o{ policy_decisions : "assessment_id"
    jobs |o--o{ policy_decisions : "job_id"
    scope_versions ||--o{ policy_decisions : "scope_version_id"
    approvals |o--o{ policy_decisions : "approval_id"
    programs ||--o{ approvals : "program_id"
    assessments ||--o{ approvals : "assessment_id"
    users ||--o{ approvals : "requested_by_user_id"
    users |o--o{ approvals : "decided_by"
    scope_versions ||--o{ approvals : "scope_version_id"
    users |o--o{ approvals : "revoked_by"
```

## Relationships that are not drawn

- `audit_events.entity_type` and `entity_id` are a soft reference with no foreign key (DB-16). The typed columns `program_id`, `assessment_id`, `job_id`, `scope_version_id`, `decision_id` and `approval_id` are real foreign keys.
- `reauth_grants` are referenced by the tables whose action consumed a grant (`policy_settings`, `scope_versions`, `refused_exclusions`, `lab_registrations`, `approvals`, `finding_verifications`, `program_provider_enablements`, `passive_source_enablements`, `tool_configs`). They are drawn in the identity and the owning domain diagrams where both ends appear.
- Views: `finding_report_status` derives each finding's report status from `report_findings` and `reports.state`. It is not a table (DB-18).
