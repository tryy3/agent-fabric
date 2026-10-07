# Projects & threads

## Sub-features
`/v1/projects` CRUD, `/v1/threads` (+ `/messages`, `/captures`), project workspaces, checkpoints.

## How to get to it (user POV)
Flutter sidebar → projects → threads; chat transcript restores after refresh.

## Driving it with curl
`POST /v1/projects`, `POST /v1/threads -d '{}'`, GET lists. Proof: created rows are returned by GET and survive plane restart (`down` not needed: kill + start the plane only).

## Gotchas
Not yet driven by this skill. Workspace volumes need Docker/Podman unless the environment kind is `local`.
