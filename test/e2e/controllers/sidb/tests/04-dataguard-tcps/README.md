# sidb/test04 — same-cluster Data Guard with TDE + TCPS

Same flow as `03-dataguard` (primary → standby → broker) with:

- TDE wallet Secret (+ `standby-wallet.zip` export before standby create)
- TCPS enabled (ClusterIP TCP 1521 + TCPS 2484)
- Longer broker Unhealthy grace for WARNING→SUCCESS

Invoked by `dataguard/test02`.
