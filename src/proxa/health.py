import anyio
import logging
import time
import psutil  # Asegúrate de importar psutil
from typing import Dict, Any
import msgspec
from proxa.types import HealthResponse, WorkerMetrics, ErrorResponse
from proxa.process import WorkerInfo

logger = logging.getLogger(__name__)

async def run_health_server(
    host: str, 
    port: int, 
    active_workers: Dict[int, WorkerInfo],
    task_status=anyio.TASK_STATUS_IGNORED
):
    """
    Inicia un servidor HTTP simple para exponer métricas y estado de salud.
    """
    logger.info(f"Iniciando servidor de health en {host}:{port}")
    
    async def handle_client(client: anyio.abc.SocketStream):
        try:
            # Leer solicitud HTTP (básica, solo para demostración)
            request_data = await client.receive()
            request_line = request_data.split(b'\r\n')[0].decode('utf-8')
            
            # Extraer ruta solicitada
            path = request_line.split(' ')[1]
            logger.debug(f"Solicitud recibida para {path}")
            
            # Preparar respuesta
            if path == '/health':
                # Endpoint básico de salud
                health_data = HealthResponse(
                    status='healthy',
                    workers=len(active_workers),
                    timestamp=time.time()
                )
                response_body = msgspec.json.encode(health_data)
                status = "200 OK"
                content_type = "application/json"
            elif path == '/metrics':
                # Endpoint detallado de métricas
                worker_metrics = [
                    WorkerMetrics(
                        pid=pid,
                        cpu_percent=worker.cpu_percent if hasattr(worker, 'cpu_percent') else None,
                        memory_mb=worker.memory_mb if hasattr(worker, 'memory_mb') else None,
                        last_update=worker.last_update if hasattr(worker, 'last_update') else None
                    )
                    for pid, worker in active_workers.items()
                ]
                metrics = {
                    'workers': len(active_workers),
                    'worker_metrics': worker_metrics,
                    'system_load': {
                        'cpu_percent': psutil.cpu_percent(),
                        'memory_percent': psutil.virtual_memory().percent,
                        'connections': len(psutil.net_connections())
                    }
                }
                response_body = msgspec.json.encode(metrics)
                status = "200 OK"
                content_type = "application/json"
            elif path == '/debug':
                # Endpoint de depuración para solución de problemas
                debug_info = {
                    'active_workers': [
                        {
                            'pid': pid,
                            'port': worker.port if hasattr(worker, 'port') else None,
                            'running': worker.psutil_process.is_running() if hasattr(worker, 'psutil_process') and worker.psutil_process else False,
                            'status': worker.psutil_process.status() if hasattr(worker, 'psutil_process') and worker.psutil_process and worker.psutil_process.is_running() else None
                        }
                        for pid, worker in active_workers.items()
                    ],
                    'system_info': {
                        'cpu_count': psutil.cpu_count(),
                        'memory_total': psutil.virtual_memory().total,
                        'memory_available': psutil.virtual_memory().available
                    }
                }
                response_body = msgspec.json.encode(debug_info)
                status = "200 OK"
                content_type = "application/json"
            else:
                # Ruta no encontrada
                error = ErrorResponse(
                    error='Not Found',
                    available_endpoints=['/health', '/metrics', '/debug']
                )
                response_body = msgspec.json.encode(error)
                status = "404 Not Found"
                content_type = "application/json"
            
            # Construir respuesta HTTP
            response = (
                f"HTTP/1.1 {status}\r\n"
                f"Content-Type: {content_type}\r\n"
                f"Content-Length: {len(response_body)}\r\n"
                f"\r\n"
            ).encode('utf-8') + response_body
            
            # Enviar respuesta
            await client.send(response)
        except Exception as e:
            logger.error(f"Error al manejar cliente en health server: {e}")
        finally:
            await client.aclose()
    
    try:
        # Crear servidor TCP
        listener = await anyio.create_tcp_listener(
            local_host=host,
            local_port=port
        )
        
        # Notificar que el servidor está listo
        task_status.started()
        logger.info(f"Servidor de health/métricas iniciado en http://{host}:{port}")
        
        # Aceptar conexiones
        async with listener:
            await listener.serve(handle_client)
    except Exception as e:
        logger.error(f"Error al iniciar servidor de health: {e}")
        raise