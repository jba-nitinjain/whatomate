# Docker Hub Publishing

This project must always publish the application image as:

- `nikyjain/whatomate:latest`

Publishing must always be multi-arch for:

- `linux/amd64`
- `linux/arm64`

The production image is built from [docker/Dockerfile](/e:/xampp/htdocs/bu-so/whatomate/docker/Dockerfile).

## Quick Command

From the repo root:

```bash
make docker-push
```

Equivalent raw command:

```bash
docker buildx build \
  --builder multiarch-builder \
  --platform linux/amd64,linux/arm64 \
  -f docker/Dockerfile \
  --cache-from type=registry,ref=nikyjain/whatomate:buildcache \
  --cache-to type=registry,ref=nikyjain/whatomate:buildcache,mode=max,oci-mediatypes=true,image-manifest=true,compression=gzip \
  -t nikyjain/whatomate:latest \
  --push .
```

## Standard Workflow

1. Log in to Docker Hub.

```bash
docker login
```

2. Make sure Buildx is available.

```bash
docker buildx version
```

3. Create or select a multi-arch builder if needed.

```bash
docker buildx create --name whatomate-multiarch --use
docker buildx inspect --bootstrap
```

If the builder already exists:

```bash
docker buildx use whatomate-multiarch
docker buildx inspect --bootstrap
```

4. Build and push the image.

```bash
make docker-push
```

This push also refreshes a registry-backed Buildx cache at `nikyjain/whatomate:buildcache` to speed up later multi-arch publishes.
The cache exporter must stay on the explicit OCI image-manifest path with gzip compression, because the default exporter settings can fail on Docker Hub with a `400 Bad request` during cache blob commit.

5. Verify the published manifest.

```bash
make docker-manifest
```

The manifest should list both `linux/amd64` and `linux/arm64`.

## Optional Version Tag

If you want a second immutable tag, push it together with `latest`, but `latest` must always be included.

```bash
docker buildx build \
  --builder multiarch-builder \
  --platform linux/amd64,linux/arm64 \
  -f docker/Dockerfile \
  --cache-from type=registry,ref=nikyjain/whatomate:buildcache \
  --cache-to type=registry,ref=nikyjain/whatomate:buildcache,mode=max,oci-mediatypes=true,image-manifest=true,compression=gzip \
  -t nikyjain/whatomate:latest \
  -t nikyjain/whatomate:2026-04-09 \
  --push .
```

## Rules

- Do not publish the app image to a different Docker Hub repository.
- Do not publish single-arch images for release.
- Do not skip the `latest` tag.
- Use [docker/Dockerfile](/e:/xampp/htdocs/bu-so/whatomate/docker/Dockerfile) for the app image.
- Keep the Buildx cache tag at `nikyjain/whatomate:buildcache` unless there is a deliberate reason to rotate it.
- Keep the cache exporter flags `oci-mediatypes=true,image-manifest=true,compression=gzip` when writing the remote Buildx cache.
- The local compose file at [docker/docker-compose.yml](/e:/xampp/htdocs/bu-so/whatomate/docker/docker-compose.yml) is for local builds and does not change the Docker Hub publishing target.

## Fast Checklist

- `docker login`
- `docker buildx inspect --bootstrap`
- `make docker-push`
- `make docker-manifest`
