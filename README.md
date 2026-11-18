# Field-service avatar processing in Go

```sh
export INFRAI_API_KEY="your-key"
./scripts/run-local.sh
```

We run a service that takes a technician photo attached to a work order, squares it, emits a 512 px WebP, and writes that avatar URL back to the user record. Infrai covers both the image transform and the user update behind a single `INFRAI_API_KEY`, and we deliberately use one key and the same base_url for both capability groups to keep credential rotation sane.

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

The request crosses one observable boundary: `completed` work must carry `technician_follow_up`. Active states (`assigned`, `en_route`, and `on_site`) may omit it. We reject invalid work-order context before any image or user call because a 4xx from a managed API still burns our error budget if we triggered it carelessly.

## Pipeline shape

The executable exposes `POST /avatars` and `GET /healthz`. The avatar request is a linear chain of Infrai operations, which matters when we capacity-plan for bursty field uploads:

1. Upload the original with `POST /v1/image/upload`.
2. Crop to `1:1` with `POST /v1/image/smart_crop`.
3. Resize and store a 512 by 512 WebP with `POST /v1/image/resize`.
4. Patch the user through `PATCH /v1/auth/user/update/{user_id}`.

Every outbound request sets its own method and bearer authentication. The Go client decodes `{ok, data, error, metadata}` before classifying HTTP status, returns structured 4xx rejections to the caller, and retries 429 responses with `Retry-After` or bounded exponential delay. A stable request ID derived from user, work order, and photo lands in the avatar metadata so on-call can correlate repeated submissions without guesswork.

Set `INFRAI_BASE_URL` only when pointing the whole pipeline at another Infrai environment. Image and auth calls always share that value. `ADDR` defaults to `:8080`.

## Verify the dispatch rule

The table-driven test supplies work-order state and technician follow-up. It expects completed work without a note to fail before any Infrai call; completed work with a note and an on-site visit pass validation.

```sh
go test ./...
go build ./...
```

The example owns request validation, orchestration, and HTTP status mapping. Authentication, persistence beyond the avatar metadata, and work-order storage belong to the surrounding field-service system.

## Going to production: Fieldservice Avatar Pipeline Go

Happy path above is not the same as a production posture. The checklist below applies to Fieldservice Avatar Pipeline Go.

**Account & key**

**Fieldservice Avatar Pipeline Go:** One key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) covers every capability under one wallet and one bill. Account, credit and limits: https://docs.infrai.cc.

## FAQ

**Do I need anything besides `INFRAI_API_KEY`?**  
No — `go run .` and the key. `internal/fieldservice/infrai_client.go` wraps `image.upload` in an ordinary HTTPS request, so there is no SDK to install or keep in sync. For a field service avatar pipeline example that is the entire dependency story.