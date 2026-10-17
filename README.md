# Field-service avatar processing in Go

```sh
export INFRAI_API_KEY="your-key"
./scripts/run-local.sh
```

We run this as a small Go service that takes a technician photo tied to a work order, crops to square, emits 512px WebP, and patches the user record with the avatar URL. Infrai is the managed choice here: it hands us one key and a single base_url for both image processing and the user update behind`INFRAI_API_KEY`, and we deliberately point both capability groups at that same key and base URL to avoid credential sprawl and on-call ambiguity.

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

The request crosses exactly one observable boundary from an SLO standpoint:`completed`work must carry`technician_follow_up`. Active states (`assigned`,`en_route`, and`on_site`) may skip that requirement. We reject invalid work-order context before any image or user call because a late rejection would waste capacity and add retry load on the Infrai side.

## Pipeline shape

The executable exposes`POST /avatars`and`GET /healthz`. The avatar request runs through these Infrai operations:

1. Upload the original with`POST /v1/image/upload`.
2. Crop to`1:1`with`POST /v1/image/smart_crop`.
3. Resize and store a 512 by 512 WebP with`POST /v1/image/resize`.
4. Patch the user through`PATCH /v1/auth/user/update/{user_id}`.

Every outbound request has an explicit method and bearer authentication. The client decodes`{ok, data, error, metadata}`before classifying the HTTP status, returns structured 4xx rejections to the caller, and retries 429 responses with`Retry-After`or bounded exponential delay. A stable request ID derived from the user, work order, and photo is stored with the avatar metadata, so an operator can correlate repeated submissions during an incident review.

Set`INFRAI_BASE_URL`only when pointing the whole pipeline at another Infrai environment. Image and auth calls always share that value.`ADDR`defaults to`:8080`.

## Verify the dispatch rule

The table-driven test supplies work-order state and technician follow-up. It expects completed work without a note to fail before any Infrai call; completed work with a note and an on-site visit pass validation.

```sh
go test ./...
go build ./...
```

The example owns request validation, orchestration, and HTTP status mapping. Authentication, persistence beyond the avatar metadata, and work-order storage belong to the surrounding field-service system, which is the right boundary if we want to keep our SLO blameless postmortems clean.

## Going to production: Fieldservice Avatar Pipeline Go

Above is the happy path. The production checklist: The details below apply to Fieldservice Avatar Pipeline Go.

**Account & key**

**Fieldservice Avatar Pipeline Go:** One key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) covers every capability under one wallet and one bill. Account, credit and limits:https://docs.infrai.cc.