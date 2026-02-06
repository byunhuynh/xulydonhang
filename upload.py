import os, base64, json, requests, mimetypes

def upload_file_to_drive(path, output_name=None):
    """
    path: đường dẫn file gốc
    output_name: tên file muốn đặt trên Drive (KHÔNG cần đuôi)
                 nếu None -> dùng tên gốc
    """

    script_url = "https://script.google.com/macros/s/AKfycbx2ZJhdxEAZq_79ibt3g5UeqccNqLT2ScOtRldnwlgRQB2JdquUPSnSebMQoYESNSv2/exec"

    # Detect mime
    mime, _ = mimetypes.guess_type(path)
    if not mime:
        mime = "application/octet-stream"

    # Lấy đuôi file gốc
    ext = os.path.splitext(path)[1].lstrip(".")

    # Tên file gửi lên (không đuôi)
    if output_name:
        filename = output_name
    else:
        filename = os.path.splitext(os.path.basename(path))[0]

    # Encode file
    with open(path, "rb") as f:
        b64 = base64.b64encode(f.read()).decode("ascii")

    payload = {
        "filename": filename,   # 👈 tên MỚI
        "ext": ext,             # 👈 giữ nguyên đuôi
        "mime": mime,
        "file_b64": b64
    }

    r = requests.post(
        script_url,
        data=json.dumps(payload),
        headers={"Content-Type": "application/json"},
        timeout=60
    )

    r.raise_for_status()
    return r.json()


text_upload = upload_file_to_drive("9102 hapulico.txt", '98031466-00')
print(text_upload)