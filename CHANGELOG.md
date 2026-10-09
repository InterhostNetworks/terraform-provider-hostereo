## 0.1.0 (unreleased)

FEATURES:

* **New Resource:** `hostereo_instance`: create (async, idempotent), resize in place (`plan_id`), power (`power_state`), project and tag assignment, import. `password` is write-only. `deletion_protection` is read-only and server-side (on by default); destroy needs it turned off in the Hostereo console and a key with `instances:delete`.
* **New Resource:** `hostereo_project`, `hostereo_tag`: create, update, import. The API has no delete, so destroy only removes them from state (with a warning).
* **New Data Sources:** `hostereo_regions`, `hostereo_plans`, `hostereo_images`, `hostereo_applications`, `hostereo_instance`.
* HTTP client retries 429 (honouring `Retry-After`) and transient 5xx on safe requests; creates carry an `Idempotency-Key`.
