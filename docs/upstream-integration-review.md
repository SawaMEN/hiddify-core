# Upstream integration review

Upstream: hiddify/hiddify-core main at b097daadd89d2b62e30c0f4353f55a973118514c (v5.4.0).

The merge uses Git -X ours: nonconflicting upstream changes are included; conflicting hunks retain the SawaMEN implementation. These paths require careful manual review before any main-branch merge.

Conflicted paths:
- `hiddify-sing-box`
- `v2/config/builder_dns_outbound_test.go`
- `v2/config/panel_uri_test.go`
- `v2/profile/test/profile_test.go`
