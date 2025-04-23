import anyio
import logging
import time
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
    Starts a simple HTTP server to expose metrics and health status.
    """
    from anyio.streams.stapled import StapledByteStream
    
    async def handle_client(client: StapledByteStream):
        # Read HTTP request (basic, for demonstration only)
        request_data = await client.receive()
        request_line = request_data.split(b'\r\n')[0].decode('utf-8')
        
        # Extract requested path
        path = request_line.split(' ')[1]
        
        # Prepare response
        if path == '/health':
            # Basic health endpoint
            health_data = HealthResponse(
                status='healthy',
                workers=len(active_workers),
                timestamp=time.time()
            )
            response_body = msgspec.json.encode(health_data)
            status = "200 OK"
        elif path == '/metrics':
            # Detailed metrics endpoint
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
                'worker_metrics': worker_metrics
            }
            response_body = msgspec.json.encode(metrics)
            status = "200 OK"
        else:
            # Path not found
            error = ErrorResponse(
                error='Not Found',
                available_endpoints=['/health', '/metrics']
            )
            response_body = msgspec.json.encode(error)
            status = "404 Not Found"
        
        # Build HTTP response
        response = (
            f"HTTP/1.1 {status}\r\n"
            f"Content-Type: application/json\r\n"
            f"Content-Length: {len(response_body)}\r\n"
            f"\r\n"
        ).encode('utf-8') + response_body
        
        # Send response
        await client.send(response)
        await client.aclose()
    
    async def serve():
        # Crear servidor TCP
        listeners = await anyio.create_tcp_listener(local_host=host, local_port=port)
        task_status.started()
        logger.info(f"Servidor de salud/métricas iniciado en http://{host}:{port}")
        
        # Aceptar conexiones
        async with listeners:
            await listeners.serve(handle_client)
    
    # Iniciar el servidor
    await serve()