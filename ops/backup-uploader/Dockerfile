FROM golang:1.25-bookworm AS backup-uploader-builder
WORKDIR /src/backend-go
COPY backend-go/go.mod backend-go/go.sum ./
RUN go mod download
COPY backend-go/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/backup-uploader-db ./cmd/backup-uploader

FROM python:3.11-slim-bookworm

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

RUN apt-get update \
    && apt-get install --no-install-recommends -y bash ca-certificates coreutils gzip \
    && rm -rf /var/lib/apt/lists/*

COPY ops/backup-uploader/requirements.txt /tmp/requirements.txt
RUN python -m pip install --no-cache-dir --disable-pip-version-check -r /tmp/requirements.txt \
    && rm -f /tmp/requirements.txt

COPY ops/backup-upload-object-storage.sh /usr/local/bin/backup-upload-object-storage.sh
COPY --from=backup-uploader-builder /out/backup-uploader-db /usr/local/bin/backup-uploader-db
RUN chmod 0755 /usr/local/bin/backup-upload-object-storage.sh
RUN chmod 0755 /usr/local/bin/backup-uploader-db

ENTRYPOINT ["/usr/local/bin/backup-upload-object-storage.sh"]
