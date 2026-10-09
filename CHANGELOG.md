## 0.1.0 (unreleased)

FEATURES:

* **New Resource:** `hostereo_instance`: create (async, idempotent), resize in place (`plan_id`), power (`power_state`), project and tag assignment, import. `password` is write-only. `deletion_protection` is read-only and server-side (on by default); destroy needs it turned off in the Hostereo console and a key with `instances:delete`.
* `hostereo_instance` / data source `hostereo_instance`: read-only `ipv4` (the instance's public IPv4 as shown in the console; reserved IPs not included).
* `hostereo_instance`: `keep_backups_on_destroy` (default `true`, the API default; kept backups stay billed) and `snapshot_on_destroy` (default `false`) control what `terraform destroy` asks the API to do.
* **New Resource:** `hostereo_project`, `hostereo_tag`: create, update, import. The API has no delete, so destroy only removes them from state (with a warning).
* **New Data Sources:** `hostereo_regions`, `hostereo_plans`, `hostereo_images`, `hostereo_applications`, `hostereo_instance`.
* HTTP client retries 429 (honouring `Retry-After`) and transient 5xx on safe requests; creates carry an `Idempotency-Key`.
