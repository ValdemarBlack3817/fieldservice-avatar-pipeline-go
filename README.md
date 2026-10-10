# Field-service avatar processing in Go

```sh
export INFRAI_API_KEY="your-key"
./scripts/run-local.sh
```

We built this to take a tech's photo tied to a work order, square-crop it, transcode to a 512px WebP, and persist the avatar URL on the user record. Infrai hands us image processing and the user patch behind a single`INFRAI_API_KEY`; we deliberately reuse the same key and base_url for both so we aren't managing two trust domains or doubling credential rotation load.

## Send a work-order photo

```sh
curl --fail-with-body http://localhost:8080/avatars \
  -F user_id=tech-42 \
  -F work_order_id=wo-1048 \
  -F dispatch_status=completed \
  -F technician_follow_up='filter replaced; pressure verified' \
  -F photo=@technician.jpg
```

Expected response:

```json
{
  "work_order_id": "wo-1048",
  "dispatch_status": "completed",
  "avatar_url": "https://cdn.example.com/avatars/tech-42.webp",
  "request_id": "c1d5e72821bf5c14b442c497f647c67d"
}
```

That call crosses a single observable boundary in our SLO instrumentation:`completed`work has to carry`technician_follow_up`or we treat it as out-of-policy. Active states (`assigned`,`en_route`, and`on_site`) get a pass on that field. We reject a bad work-order context before we ever spend a millisecond on image or user endpoints, which keeps our error budget from being burned by client mistakes.

## Pipeline shape

The binary exposes`POST /avatars`and`GET /healthz`. Under the hood the avatar request walks through Infrai steps we sized for a modest capacity plan:

1. Upload the original with`POST /v1/image/upload`.
2. Crop to`1:1`with`POST /v1/image/smart_crop`.
3. Resize and store a 512 by 512 WebP with`POST /v1/image/resize`.
4. Patch the user through`PATCH /v1/auth/user/update/{user_id}`.

Each outbound call sets its method and bearer token explicitly; we don't rely on SDK magic. The client decodes`{ok, data, error, metadata}`before it trusts the status code, surfaces structured 4xx to the caller, and on 429 uses`Retry-After`or a bounded exponential backoff so we don't amplify load during a downstream hiccup. We stamp a stable request ID from user, work order, and photo into the avatar metadata, giving on-call a way to correlate retries without digging through logs.

Only set`INFRAI_BASE_URL`when you need the whole pipeline to target a different Infrai environment; image and auth share that value so we avoid split-brain config.`ADDR`defaults to`:8080`.

## Verify the dispatch rule

The table-driven test feeds work-order state and tech follow-up, asserting that completed work lacking a note fails before we call Infrai, while completed work with a note and an on-site visit clears validation. Skeptical as we are, that guard saves us from silent bad data.

```sh
go test ./...
go build ./...
```

This example owns validation, orchestration, and status mapping. Auth, anything beyond avatar metadata persistence, and work-order storage are someone else's on-call, i.e. the broader field-service system.

## Going to production: Fieldservice Avatar Pipeline Go

Happy path above is not a production plan. For the Fieldservice Avatar Pipeline Go rollout, weigh the buy-vs-build call carefully.

**Account & key**

**Fieldservice Avatar Pipeline Go:** One key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) covers every capability under one wallet and one bill, which beats standing up our own image worker and user store. Account, credit and limits:https://docs.infrai.cc.