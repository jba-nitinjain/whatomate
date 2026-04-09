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
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -f docker/Dockerfile \
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
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -f docker/Dockerfile \
  -t nikyjain/whatomate:latest \
  --push .
```

5. Verify the published manifest.

```bash
docker buildx imagetools inspect nikyjain/whatomate:latest
```

The manifest should list both `linux/amd64` and `linux/arm64`.

## Optional Version Tag

If you want a second immutable tag, push it together with `latest`, but `latest` must always be included.

```bash
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -f docker/Dockerfile \
  -t nikyjain/whatomate:latest \
  -t nikyjain/whatomate:2026-04-09 \
  --push .
```

## Rules

- Do not publish the app image to a different Docker Hub repository.
- Do not publish single-arch images for release.
- Do not skip the `latest` tag.
- Use [docker/Dockerfile](/e:/xampp/htdocs/bu-so/whatomate/docker/Dockerfile) for the app image.
- The local compose file at [docker/docker-compose.yml](/e:/xampp/htdocs/bu-so/whatomate/docker/docker-compose.yml) is for local builds and does not change the Docker Hub publishing target.

## Fast Checklist

- `docker login`
- `docker buildx use whatomate-multiarch`
- `docker buildx inspect --bootstrap`
- build from [docker/Dockerfile](/e:/xampp/htdocs/bu-so/whatomate/docker/Dockerfile)
- push `nikyjain/whatomate:latest`
- verify with `docker buildx imagetools inspect nikyjain/whatomate:latest`
