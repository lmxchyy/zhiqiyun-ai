#!/usr/bin/env python3
import contextlib
import json
import logging
import os
import secrets
import sys
import time
from pathlib import Path

RESULT_PREFIX = "__XIANZHI_SEEDANCE_RESULT__"


def _quiet_logs() -> None:
    logging.basicConfig(level=logging.WARNING)
    try:
        from loguru import logger

        logger.remove()
    except Exception:
        pass


def _extend_sys_path() -> None:
    for key in ("CME_SEEDANCE_SDK_PATH", "MAAS_SEEDANCE_SDK_PATH"):
        value = os.environ.get(key, "").strip()
        if value:
            sys.path.insert(0, value)
    deps = os.environ.get("CME_SEEDANCE_DEPS_PATH", "").strip()
    if deps:
        sys.path.insert(0, deps)


def _text(value, fallback=""):
    if value is None:
        return fallback
    text = str(value).strip()
    return text if text else fallback


def _number(value, fallback):
    try:
        number = int(value)
        return number if number > 0 else fallback
    except Exception:
        return fallback


def _bool(value, fallback=False):
    if isinstance(value, bool):
        return value
    if value is None:
        return fallback
    if isinstance(value, (int, float)):
        return value != 0
    text = str(value).strip().lower()
    if text in ("1", "true", "yes", "on"):
        return True
    if text in ("0", "false", "no", "off"):
        return False
    return fallback


def _image_items(image_urls):
    items = []
    if not isinstance(image_urls, list):
        return items
    for url in image_urls:
        text = _text(url)
        if not text:
            continue
        items.append(
            {
                "type": "image_url",
                "image_url": {"url": text},
                "role": "reference_image",
            }
        )
    return items


def _quiet_call(fn, *args, **kwargs):
    with contextlib.redirect_stdout(sys.stderr):
        return fn(*args, **kwargs)


def _emit_result(payload) -> None:
    sys.__stdout__.write(RESULT_PREFIX + json.dumps(payload, ensure_ascii=False) + "\n")
    sys.__stdout__.flush()


def main() -> int:
    req = json.load(sys.stdin)
    output_dir = Path(_text(req.get("outputDir"), "data/generated-media")).expanduser()
    output_dir.mkdir(parents=True, exist_ok=True)
    output_dir = output_dir.resolve()
    log_dir = output_dir / "seedance_logs"
    log_dir.mkdir(parents=True, exist_ok=True)
    os.environ.setdefault("LOG_DIR", str(log_dir))
    os.environ.setdefault("LOGDIR", str(log_dir))
    os.chdir(output_dir)

    _quiet_logs()
    _extend_sys_path()
    with contextlib.redirect_stdout(sys.stderr):
        from maas_seedance import MaasSeedanceClient

    base_url = _text(req.get("baseUrl"))
    api_key = _text(req.get("apiKey"))
    model = _text(req.get("model"), "doubao-seedance-2.0")
    prompt = _text(req.get("prompt"))
    params = req.get("params") if isinstance(req.get("params"), dict) else {}
    if not base_url or not api_key or not prompt:
        raise ValueError("baseUrl, apiKey and prompt are required")

    key_dir = output_dir / "seedance_keys"
    key_dir.mkdir(parents=True, exist_ok=True)
    public_key_path = key_dir / "seedance_pub.pem"
    private_key_path = key_dir / "seedance_priv.pem"

    timeout_seconds = _number(req.get("timeoutSeconds"), 900)
    duration = _number(params.get("duration"), 5)
    ratio = _text(params.get("ratio"), "16:9")
    resolution = _text(params.get("resolution"), "720p")
    generate_audio = _bool(params.get("generate_audio", params.get("generateAudio")), False)

    client = _quiet_call(
        MaasSeedanceClient,
        maas_base_url=base_url,
        maas_api_key=api_key,
        maas_model=model,
        enable_video_encrypt=True,
    )
    _quiet_call(client.set_video_file_encrypt_key, str(public_key_path), str(private_key_path))

    content = [{"type": "text", "text": prompt}]
    content.extend(_image_items(req.get("imageUrls")))
    request_data = {
        "content": content,
        "generate_audio": generate_audio,
        "ratio": ratio,
        "duration": duration,
        "resolution": resolution,
        "watermark": False,
        "return_last_frame": False,
    }

    task_id = _quiet_call(client.create_video_generation_task, request_data)
    if not task_id:
        raise RuntimeError("create_video_generation_task returned empty task id")

    deadline = time.time() + timeout_seconds
    task_info = {}
    while time.time() < deadline:
        time.sleep(10)
        task_info = _quiet_call(client.query_video_generation_task, task_id) or {}
        status = _text(task_info.get("status")).lower()
        if status == "succeeded":
            filename = f"seedance-{task_id}-{secrets.token_hex(4)}.mp4"
            output_path = output_dir / filename
            ok = _quiet_call(client.download_video, task_id, str(output_path))
            if not ok or not output_path.exists() or output_path.stat().st_size <= 0:
                raise RuntimeError("download_video failed")
            public_base = _text(req.get("publicURLBase"), "/api/v1/generated-media/")
            if not public_base.endswith("/"):
                public_base += "/"
            result = {
                "provider": "cmecloud-seedance",
                "providerTaskId": task_id,
                "status": "succeeded",
                "videoUrl": public_base + filename,
                "localPath": str(output_path),
                "actualModel": _text(task_info.get("model")),
                "raw": task_info,
                "metadata": {
                    "duration": task_info.get("duration", duration),
                    "ratio": task_info.get("ratio", ratio),
                    "resolution": task_info.get("resolution", resolution),
                    "framespersecond": task_info.get("framespersecond"),
                },
            }
            _emit_result(result)
            return 0
        if status == "failed":
            raise RuntimeError(json.dumps(task_info, ensure_ascii=False))

    raise TimeoutError(json.dumps(task_info, ensure_ascii=False))


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:
        print(json.dumps({"status": "failed", "error": str(exc)}, ensure_ascii=False), file=sys.stderr)
        raise
