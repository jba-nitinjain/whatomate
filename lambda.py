import os
import json
import requests
import pymysql
import logging
import inspect  # For line numbers
from datetime import datetime
import urllib.request
import urllib.error

# Logger setup
logging.basicConfig(level=logging.INFO, force=True)
logger = logging.getLogger()
logger.setLevel(logging.INFO)
logger.info(f"[Line {inspect.currentframe().f_lineno}] Lambda module import started.")

# Fetch environment variables for RDS configuration
RDS_CONFIG = {
    "host": os.environ['DB_HOST'],
    "user": os.environ['DB_USER'],
    "password": os.environ['DB_PASSWORD'],
    "database": os.environ['DB_NAME']
}
logger.info(
    f"[Line {inspect.currentframe().f_lineno}] Environment variables loaded. "
    f"has_db_host={bool(RDS_CONFIG['host'])}, "
    f"has_db_user={bool(RDS_CONFIG['user'])}, "
    f"has_db_name={bool(RDS_CONFIG['database'])}, "
    f"has_whatomate_token={bool(os.environ.get('WHATOMATE_API_TOKEN'))}, "
    f"has_whatomate_account_name={bool(os.environ.get('WHATOMATE_ACCOUNT_NAME'))}"
)

# Global variable to cache the connection across Lambda invocations
rds_connection = None
logger.info(f"[Line {inspect.currentframe().f_lineno}] Lambda module import completed. rds_connection initialized to None.")

def connect_to_rds():
    """Establish a connection to RDS."""
    line = inspect.currentframe().f_lineno
    logger.info(f"[Line {line}] Connecting to RDS at {datetime.now()}")
    try:
        conn = pymysql.connect(
            host=RDS_CONFIG["host"],
            user=RDS_CONFIG["user"],
            password=RDS_CONFIG["password"],
            database=RDS_CONFIG["database"],
            cursorclass=pymysql.cursors.DictCursor,
            autocommit=False  # control commits manually
        )
        line = inspect.currentframe().f_lineno
        logger.info(f"[Line {line}] Connected to RDS successfully.")
        return conn
    except Exception as e:
        line = inspect.currentframe().f_lineno
        logger.error(f"[Line {line}] Failed to connect to RDS: {e}")
        raise

def get_rds_connection():
    """
    Retrieve the global RDS connection. If it doesn't exist or is no longer alive,
    reconnect.
    """
    global rds_connection
    if rds_connection is None:
        rds_connection = connect_to_rds()
    else:
        try:
            # Ping to check if connection is still alive.
            # 'reconnect=True' will try to re-establish if necessary.
            rds_connection.ping(reconnect=True)
        except Exception as e:
            logger.error(f"RDS connection lost, reconnecting: {e}")
            rds_connection = connect_to_rds()
    return rds_connection

def db_logging(cursor, data, file, phone_number_id):
    """
    Log data to the log_data table, including line number.
    """
    line = inspect.currentframe().f_back.f_lineno  # Capture caller's line number
    created_datetime = datetime.now().isoformat()
    try:
        query = """
            INSERT INTO log_data (file, line, Phone_Number_ID, output, createdDatetime)
            VALUES (%s, %s, %s, %s, %s)
        """
        # Convert data to JSON string
        output = json.dumps(data, default=str)
        cursor.execute(query, (file, line, phone_number_id, output, created_datetime))
        logger.info(f"[Line {line}] Logged data to 'log_data' table at {created_datetime}.")
    except Exception as e:
        logger.error(f"[Line {line}] Failed to log data: {e}")

def sync_to_whatomate(body_data, wamid):
    """
    Syncs the successfully sent message to Whatomate using the custom
    record-only external endpoint.

    Only this function is updated. The rest of the Lambda flow remains unchanged.
    This version improves CloudWatch visibility and logs Whatomate sync failures
    into log_data using db_logging().
    """
    line = inspect.currentframe().f_lineno
    logger.info(f"[Line {line}] Whatomate sync process started. wamid={wamid}, body_data={json.dumps(body_data, default=str)}")

    WHATOMATE_API_URL = os.environ.get(
        "WHATOMATE_API_URL",
        "https://whatomate.bu-so.com/api/messages/external"
    )
    WHATOMATE_API_KEY = os.environ.get("WHATOMATE_API_TOKEN")
    WHATOMATE_ACCOUNT_NAME = os.environ.get("WHATOMATE_ACCOUNT_NAME", "Jain Bafna & Associates")
    logger.info(
        f"[Line {line}] Whatomate config loaded. "
        f"url={WHATOMATE_API_URL}, account_name={WHATOMATE_ACCOUNT_NAME}, has_api_key={bool(WHATOMATE_API_KEY)}"
    )

    allowed_ids_str = os.environ.get("WHATOMATE_SYNC_PHONE_IDS", "116800811516809")
    allowed_phone_ids = {pid.strip() for pid in allowed_ids_str.split(",") if pid.strip()}
    logger.info(f"[Line {line}] Allowed Whatomate phone IDs loaded: {sorted(list(allowed_phone_ids))}")

    def log_sync_issue(phone_number_id, data):
        """
        Write Whatomate sync issues to log_data without changing the caller flow.
        Uses a separate DB connection so the current transaction remains untouched.
        """
        conn = None
        try:
            conn = connect_to_rds()
            with conn.cursor() as cursor:
                db_logging(cursor, data, file="sync_to_whatomate", phone_number_id=phone_number_id or "N/A")
            conn.commit()
        except Exception as log_err:
            logger.error(f"[Line {inspect.currentframe().f_lineno}] Failed to log Whatomate sync issue to DB: {log_err}")
        finally:
            try:
                if conn:
                    conn.close()
            except Exception:
                pass

    if not WHATOMATE_API_KEY:
        warning_msg = "WHATOMATE_API_TOKEN is missing. Skipping sync."
        logger.warning(f"[Line {line}] {warning_msg}")
        log_sync_issue("N/A", {
            "stage": "whatomate_sync",
            "status": "skipped",
            "reason": warning_msg
        })
        return

    try:
        credentials = body_data.get("credentials", {})
        phone_number_id = str(credentials.get("Phone_Number_ID", ""))
        logger.info(f"[Line {line}] Credentials extracted for Whatomate sync. phone_number_id={phone_number_id}")

        def extract_media_fields(media_obj):
            """
            Extract a renderable media reference for Whatomate.
            Prefers externally hosted links because Whatomate's external message
            endpoint needs a URL/path for chat preview rendering.
            """
            if not isinstance(media_obj, dict):
                return "", "", "", ""

            media_url = str(media_obj.get("link") or media_obj.get("url") or "").strip()
            media_mime_type = str(media_obj.get("mime_type") or media_obj.get("mimeType") or "").strip()
            media_filename = str(media_obj.get("filename") or "").strip()
            media_id = str(media_obj.get("id") or "").strip()

            return media_url, media_mime_type, media_filename, media_id

        if phone_number_id not in allowed_phone_ids:
            info_msg = f"Phone_Number_ID {phone_number_id} not in allowed list. Skipping Whatomate sync."
            logger.info(f"[Line {line}] {info_msg}")
            log_sync_issue(phone_number_id, {
                "stage": "whatomate_sync",
                "status": "skipped",
                "reason": info_msg
            })
            return

        message_data = body_data.get("message", {})
        logger.info(f"[Line {line}] Message data extracted for Whatomate sync: {json.dumps(message_data, default=str)}")
        if not message_data:
            warning_msg = "No message data found for Whatomate sync."
            logger.warning(f"[Line {line}] {warning_msg}")
            log_sync_issue(phone_number_id, {
                "stage": "whatomate_sync",
                "status": "skipped",
                "reason": warning_msg,
                "body_data": body_data
            })
            return

        to_number = str(message_data.get("to", "")).replace("+", "").strip()
        logger.info(f"[Line {line}] Destination number parsed for Whatomate sync. to_number={to_number}")
        if not to_number:
            warning_msg = "Recipient phone number is missing or empty. Skipping Whatomate sync."
            logger.warning(f"[Line {line}] {warning_msg}")
            log_sync_issue(phone_number_id, {
                "stage": "whatomate_sync",
                "status": "failed",
                "reason": warning_msg,
                "message_data": message_data
            })
            return

        msg_type = str(message_data.get("type", "text")).lower()
        logger.info(f"[Line {line}] Message type resolved for Whatomate sync. msg_type={msg_type}")

        # FIX: derive account name per-message so each WhatsApp number routes to
        # its own account in Whatomate instead of always using the global env var.
        whatsapp_account_name = credentials.get("whatsapp_account") or WHATOMATE_ACCOUNT_NAME

        whatomate_payload = {
            "phone_number": to_number,
            # FIX: phone_number_id must be a top-level field so Whatomate's
            # resolveExternalMessageTargetOrgAndAccount() picks it up for routing.
            # Previously it was buried inside metadata{} where the handler ignores it.
            "phone_number_id": phone_number_id,
            "whatsapp_account": whatsapp_account_name,
            "whatsapp_message_id": str(wamid),
            "external_message_id": str(body_data.get("bulk_send_id", f"aws-sqs-{wamid}")),
            "sent_at": datetime.utcnow().isoformat() + "Z",
            "metadata": {
                "source": "external_api",
                "source_system": "aws_lambda",
                "phone_number_id": phone_number_id,
                "bulk_send_id": str(body_data.get("bulk_send_id", "")),
                "original_message_type": msg_type
            }
        }

        if msg_type == "template":
            template_data = message_data.get("template", {}) or {}
            components = template_data.get("components", []) or []
            logger.info(f"[Line {line}] Processing template message for Whatomate sync. template_data={json.dumps(template_data, default=str)}")

            template_params = {}
            header_media_url = ""
            header_media_mime_type = ""
            header_media_filename = ""
            header_media_id = ""
            idx = 1
            for component in components:
                if not isinstance(component, dict):
                    continue
                component_type = str(component.get("type", "")).lower()
                parameters = component.get("parameters", []) or []
                for param in parameters:
                    if not isinstance(param, dict):
                        continue

                    if component_type == "header":
                        param_type = str(param.get("type", "")).lower()
                        if param_type in ("image", "video", "document"):
                            media_obj = param.get(param_type, {}) or {}
                            media_url, media_mime_type, media_filename, media_id = extract_media_fields(media_obj)
                            if media_url and not header_media_url:
                                header_media_url = media_url
                            if media_mime_type and not header_media_mime_type:
                                header_media_mime_type = media_mime_type
                            if media_filename and not header_media_filename:
                                header_media_filename = media_filename
                            if media_id and not header_media_id:
                                header_media_id = media_id

                    value = param.get("text")
                    if value is None:
                        value = param.get("payload")
                    if value is not None:
                        template_params[str(idx)] = str(value)
                        idx += 1

            template_name = template_data.get("name", "external_template")
            whatomate_payload["type"] = "template"
            whatomate_payload["template_name"] = template_name
            whatomate_payload["template_params"] = template_params
            whatomate_payload["content"] = {"body": f"[Template: {template_name}]"}
            if header_media_url:
                whatomate_payload["header_media_url"] = header_media_url
            if header_media_mime_type:
                whatomate_payload["header_media_mime_type"] = header_media_mime_type
            if header_media_filename:
                whatomate_payload["header_media_filename"] = header_media_filename
            if header_media_id:
                whatomate_payload["metadata"]["template_header_media_id"] = header_media_id
            logger.info(
                f"[Line {line}] Template payload prepared for Whatomate sync. "
                f"template_name={template_name}, template_params={json.dumps(template_params, default=str)}, "
                f"header_media_url={header_media_url}, header_media_filename={header_media_filename}, "
                f"has_header_media_id={bool(header_media_id)}"
            )

        elif msg_type == "text":
            whatomate_payload["type"] = "text"
            whatomate_payload["content"] = {
                "body": message_data.get("text", {}).get("body", "External message")
            }
            logger.info(f"[Line {line}] Text payload prepared for Whatomate sync: {json.dumps(whatomate_payload, default=str)}")

        elif msg_type in ["image", "video", "audio", "document"]:
            media_data = message_data.get(msg_type, {}) or {}
            caption = media_data.get("caption", "")
            media_url, media_mime_type, media_filename, media_id = extract_media_fields(media_data)
            whatomate_payload["type"] = msg_type
            whatomate_payload["content"] = {
                "body": caption or f"[{msg_type.capitalize()}]"
            }
            if media_url:
                whatomate_payload["media_url"] = media_url
            if media_mime_type:
                whatomate_payload["media_mime_type"] = media_mime_type
            if media_filename:
                whatomate_payload["media_filename"] = media_filename
            if media_id:
                whatomate_payload["metadata"]["media_id"] = media_id
            logger.info(
                f"[Line {line}] Media payload prepared for Whatomate sync. "
                f"media_type={msg_type}, media_data={json.dumps(media_data, default=str)}, "
                f"payload={json.dumps(whatomate_payload, default=str)}"
            )

        else:
            whatomate_payload["type"] = "text"
            whatomate_payload["content"] = {
                "body": f"External message ({msg_type})"
            }
            logger.info(f"[Line {line}] Fallback payload prepared for Whatomate sync: {json.dumps(whatomate_payload, default=str)}")

        headers = {
            "X-API-Key": WHATOMATE_API_KEY,
            "Content-Type": "application/json",
            "Accept": "application/json"
        }

        logger.info(f"[Line {line}] POST {WHATOMATE_API_URL}")
        logger.info(f"[Line {line}] Whatomate payload: {json.dumps(whatomate_payload, default=str)}")

        response = requests.post(
            WHATOMATE_API_URL,
            json=whatomate_payload,
            headers=headers,
            # FIX: separate connect (5 s) and read (30 s) timeouts.
            # Previously timeout=10 used a single scalar which could silently
            # block the Lambda for up to 20 s per message under slow responses.
            timeout=(5, 30),
            allow_redirects=False
        )

        logger.info(f"[Line {line}] Whatomate status: {response.status_code}")
        logger.info(f"[Line {line}] Whatomate response headers: {dict(response.headers)}")
        logger.info(f"[Line {line}] Whatomate response: {response.text}")

        if 300 <= response.status_code < 400:
            error_data = {
                "stage": "whatomate_sync",
                "status": "failed",
                "reason": "redirect_received",
                "status_code": response.status_code,
                "location": response.headers.get("Location"),
                "response": response.text,
                "payload": whatomate_payload
            }
            logger.error(f"[Line {line}] Redirect received from Whatomate. Location: {response.headers.get('Location')}")
            log_sync_issue(phone_number_id, error_data)
        elif response.status_code in [200, 201]:
            logger.info(f"[Line {line}] Successfully synced to Whatomate.")
        else:
            error_data = {
                "stage": "whatomate_sync",
                "status": "failed",
                "reason": "non_2xx_response",
                "status_code": response.status_code,
                "response": response.text,
                "payload": whatomate_payload
            }
            logger.error(f"[Line {line}] Whatomate Sync Failed with Status {response.status_code}. Response: {response.text}")
            log_sync_issue(phone_number_id, error_data)

    except requests.exceptions.RequestException as e:
        error_data = {
            "stage": "whatomate_sync",
            "status": "failed",
            "reason": "request_exception",
            "error": str(e),
            "wamid": str(wamid)
        }
        logger.exception(f"[Line {line}] Network/HTTP Error syncing to Whatomate: {e}")
        log_sync_issue(body_data.get("credentials", {}).get("Phone_Number_ID", "N/A"), error_data)
    except Exception as e:
        error_data = {
            "stage": "whatomate_sync",
            "status": "failed",
            "reason": "unexpected_exception",
            "error": str(e),
            "wamid": str(wamid)
        }
        logger.exception(f"[Line {line}] Unexpected error syncing to Whatomate: {e}")
        log_sync_issue(body_data.get("credentials", {}).get("Phone_Number_ID", "N/A"), error_data)

def process_batch(event, conn):
    """
    Process the batch of messages from SQS trigger.
    """
    records = event.get("Records", [])
    line = inspect.currentframe().f_lineno
    logger.info(f"[Line {line}] Processing {len(records)} messages at {datetime.now()}")

    try:
        with conn.cursor() as cursor:
            for i, record in enumerate(records):
                try:
                    body = json.loads(record['body'])
                    if not validate_message_structure(body):
                        db_logging(cursor, body, file="lambda_handler", phone_number_id="N/A")
                        continue

                    message = body.get('message')
                    credentials = body.get('credentials')
                    bulk_send_id = body.get('bulk_send_id')
                    category = body.get('category')

                    line = inspect.currentframe().f_lineno
                    logger.info(f"[Line {line}] Processing message {i+1}/{len(records)}: Sending API call")

                    fb_response = send_message(message, credentials, category)

                    if fb_response['status'] == "success":
                        wamid = fb_response['message_id']

                        # Update local RDS
                        log_message_to_conversations(cursor, wamid, message, credentials, fb_response)
                        update_bulk_status(cursor, bulk_send_id, "sent", wamid)

                        # Sync to Whatomate (Added here)
                        sync_to_whatomate(body, wamid)

                    else:
                        update_bulk_status(cursor, bulk_send_id, "failed", fb_response.get('error'))
                        db_logging(cursor, {"message": message, "error": fb_response}, file="lambda_handler", phone_number_id=credentials.get("Phone_Number_ID"))
                except Exception as e:
                    logger.error(f"[Line {inspect.currentframe().f_lineno}] Error processing message {i+1}: {e}")
            conn.commit()  # Commit all changes once the batch is processed
    except Exception as e:
        conn.rollback()  # Rollback in case of any error during batch processing
        logger.error(f"Error in processing batch, rolled back changes: {e}")
        raise

def validate_message_structure(body):
    """Validate the required keys in the input structure."""
    return all(key in body for key in ['message', 'credentials', 'bulk_send_id'])

def send_message(message, credentials, category):
    """
    Send a message to Facebook Graph API.
    The endpoint changes based on the message category.
    """
    line = inspect.currentframe().f_lineno
    try:
        base_url = f"https://graph.facebook.com/{credentials['Version']}/{credentials['Phone_Number_ID']}"
        endpoint = "messages"
        if category == 'MARKETING':
            endpoint = "marketing_messages"

        fb_url = f"{base_url}/{endpoint}"

        headers = {
            "Authorization": f"Bearer {credentials['User_Access_Token']}",
            "Content-Type": "application/json"
        }

        logger.info(f"[Line {line}] Sending API call to {fb_url} with payload: {json.dumps(message, indent=2)}")

        response = requests.post(fb_url, headers=headers, json=message)
        response_data = response.json()

        # Added safety check for 'messages' array existence and length
        if 'messages' in response_data and len(response_data['messages']) > 0 and response_data['messages'][0].get('id'):
            logger.info(f"[Line {line}] Message sent successfully.")
            return {"status": "success", "message_id": response_data['messages'][0]['id']}
        else:
            logger.error(f"[Line {line}] Failed to send message: {response_data}")
            return {"status": "failed", "error": response_data}
    except Exception as e:
        logger.error(f"[Line {line}] Exception during API call: {e}")
        return {"status": "failed", "error": str(e)}

def update_bulk_status(cursor, bulk_send_id, status, message_id=None):
    """Update the bulk_send_txn table."""
    query = """
        UPDATE bulk_send_txn
        SET sent_date_time = %s, message_id = %s, status = %s
        WHERE id = %s
    """
    cursor.execute(query, (datetime.now(), message_id, status, bulk_send_id))

def log_message_to_conversations(cursor, message_id, message, credentials, fb_response):
    """Log message details into the conversations table."""
    now = datetime.now()
    query = """
        INSERT INTO conversations
        (id, direction, mobile_number, type, content, status, createdDatetime, updatedDatetime, raw_data, Phone_Number_ID)
        VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
    """
    cursor.execute(query, (
        message_id, 'sent', message['to'], message.get('type', 'text'),
        json.dumps(message), "sent", now, now,
        json.dumps(fb_response), credentials['Phone_Number_ID']
    ))

# Lambda Entry Point
def lambda_handler(event, context):
    print("LAMBDA_HANDLER_ENTERED")
    line = inspect.currentframe().f_lineno
    logger.info(f"[Line {line}] Lambda function started with {len(event.get('Records', []))} messages.")
    try:
        conn = get_rds_connection()
        process_batch(event, conn)
        logger.info(f"[Line {inspect.currentframe().f_lineno}] Lambda function completed successfully.")
        return {"status": "success", "messages_processed": len(event.get("Records", []))}
    except Exception as e:
        logger.error(f"[Line {inspect.currentframe().f_lineno}] Error in Lambda handler: {e}")
        return {"status": "error", "error": str(e)}
