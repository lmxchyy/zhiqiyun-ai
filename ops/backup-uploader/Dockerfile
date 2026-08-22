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
RUN chmod 0755 /usr/local/bin/backup-upload-object-storage.sh

ENTRYPOINT ["/usr/local/bin/backup-upload-object-storage.sh"]
