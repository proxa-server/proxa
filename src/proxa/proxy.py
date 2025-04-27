import anyio
import logging
import time
import random
import httpx
import os
import psutil  # Añadir esta importación
from typing import Dict, List, Optional, Set
import msgspec
from proxa.types import ProxyStats, ProxyConfig, RoutingTable
from proxa.process import WorkerInfo

logger = logging.getLogger(__name__)

class ProxyServer:
    """
    Proxy server that distributes requests among multiple workers.
    """
    def __init__(self, host: str = "127.0.0.1", port: int = 8000, active_workers: Dict[int, WorkerInfo] = None):
        self.host = host
        self.port = port
        self.active_workers = active_workers or {}
        self.workers_urls = set()
        self.last_worker_index = 0
        
        # Basic statistics
        self.request_count = 0
        self.error_count = 0
        self.last_request_time = 0
        
        logger.info(f"Proxy initialized at {host}:{port}")
    
    async def start(self):
        """Starts the proxy server."""
        logger.info(f"Starting proxy at {self.host}:{self.port}")
        
        try:
            # Create a TCP listener
            listener = await anyio.create_tcp_listener(
                local_host=self.host,
                local_port=self.port
            )
            
            # Start worker synchronization task
            async with anyio.create_task_group() as tg:
                tg.start_soon(self._sync_workers_task)
                
                # Serve connections
                async with listener:
                    await listener.serve(self.handle_connection)
                    
        except Exception as e:
            logger.error(f"Error starting proxy: {e}")
            raise
    
    async def _sync_workers_task(self):
        """Tarea que sincroniza periódicamente los workers disponibles."""
        while True:
            try:
                self._sync_workers()
                await anyio.sleep(5)  # Sincronizar cada 5 segundos
            except Exception as e:
                logger.error(f"Error en sincronización de workers: {e}")
                await anyio.sleep(10)  # Esperar más tiempo en caso de error
    
    def _sync_workers(self):
        """Sincroniza la lista de workers disponibles."""
        # Guardar la lista anterior para comparación
        previous_urls = self.workers_urls.copy()
        self.workers_urls = set()
        
        # Contador de workers activos
        active_count = 0
        
        # Primero, verificar workers en active_workers
        if self.active_workers:
            for pid, worker in list(self.active_workers.items()):
                try:
                    # Verificar si el worker está activo
                    is_active = False
                    if hasattr(worker, 'psutil_process') and worker.psutil_process:
                        try:
                            is_active = worker.psutil_process.is_running()
                            # Verificar también si el proceso está respondiendo (no zombie)
                            if is_active and worker.psutil_process.status() == psutil.STATUS_ZOMBIE:
                                is_active = False
                        except Exception:
                            is_active = False
                    
                    # Si el worker no está activo, omitirlo
                    if not is_active and hasattr(worker, 'psutil_process') and worker.psutil_process:
                        continue
                    
                    # Agregar el worker a la lista de URLs disponibles
                    if hasattr(worker, 'port'):
                        worker_url = f"http://127.0.0.1:{worker.port}"
                        self.workers_urls.add(worker_url)
                        active_count += 1
                        logger.debug(f"Worker sincronizado: {worker_url} (PID: {pid})")
                except Exception as e:
                    logger.warning(f"Error al sincronizar worker {pid}: {e}")
        
        # Si no se encontraron workers en active_workers, buscar en puertos conocidos
        if not self.workers_urls:
            for port in range(8001, 8011):
                worker_url = f"http://127.0.0.1:{port}"
                # No verificamos conectividad aquí para evitar bloqueos
                self.workers_urls.add(worker_url)
                logger.debug(f"Worker añadido por puerto conocido: {worker_url}")
        
        # Verificar si se agregaron nuevos workers o se eliminaron algunos
        added = self.workers_urls - previous_urls
        removed = previous_urls - self.workers_urls
        
        if added:
            logger.info(f"Nuevos workers agregados: {added}")
        if removed:
            logger.info(f"Workers eliminados: {removed}")
        
        logger.info(f"Sincronización completada: {active_count} workers activos, {len(self.workers_urls)} URLs disponibles")
        
        # Si no hay workers disponibles pero hay métricas de workers, algo está mal
        if not self.workers_urls and active_count > 0:
            logger.warning(f"Inconsistencia detectada: {active_count} workers activos pero ninguno disponible en la lista de URLs")
    
    async def handle_connection(self, client: anyio.abc.SocketStream):
        """Maneja una conexión de cliente."""
        self.request_count += 1
        self.last_request_time = time.time()
        
        try:
            # Establecer un timeout para la conexión del cliente
            with anyio.move_on_after(30.0):  # 30 segundos de timeout
                # Leer datos del cliente
                request_data = await client.receive()
                
                # Seleccionar worker
                worker_url = await self.select_worker()
                if not worker_url:
                    logger.warning("No hay workers disponibles para manejar la solicitud")
                    response = b"HTTP/1.1 503 Service Unavailable\r\nContent-Length: 25\r\n\r\nNo hay workers disponibles.\n"
                    await client.send(response)
                    return
                
                # Reenviar solicitud al worker
                response_data = await self.forward_request(request_data, worker_url)
                
                # Enviar respuesta al cliente
                await client.send(response_data)
        except TimeoutError:
            logger.error("Timeout al manejar la conexión del cliente")
            try:
                error_msg = "Timeout al procesar la solicitud"
                response = f"HTTP/1.1 504 Gateway Timeout\r\nContent-Length: {len(error_msg)}\r\n\r\n{error_msg}"
                await client.send(response.encode())
            except Exception:
                pass
        except Exception as e:
            self.error_count += 1
            logger.error(f"Error al manejar conexión: {e}")
            
            try:
                # Intentar enviar respuesta de error al cliente
                error_msg = f"Error interno del servidor: {str(e)}"
                response = f"HTTP/1.1 500 Internal Server Error\r\nContent-Length: {len(error_msg)}\r\n\r\n{error_msg}"
                await client.send(response.encode())
            except Exception as send_error:
                logger.error(f"Error al enviar respuesta de error: {send_error}")
        finally:
            # Asegurar que la conexión se cierre correctamente
            try:
                await client.aclose()
            except Exception:
                pass
            
    async def select_worker(self) -> Optional[str]:
        """Selecciona un worker para manejar la solicitud."""
        # Si no hay workers sincronizados, intentar sincronizar ahora
        if not self.workers_urls:
            self._sync_workers()
        
        # Si todavía no hay workers, intentar recuperación de emergencia
        if not self.workers_urls:
            logger.warning("Iniciando recuperación de emergencia de workers")
            await self._emergency_worker_recovery()
        
        if not self.workers_urls:
            return None
        
        # Usar round-robin simple para distribuir las solicitudes
        workers_list = list(self.workers_urls)
        if not workers_list:
            return None
        
        self.last_worker_index = (self.last_worker_index + 1) % len(workers_list)
        selected = workers_list[self.last_worker_index]
        return selected
    
    async def _emergency_worker_recovery(self):
        """Intenta recuperar workers en caso de emergencia."""
        try:
            # Verificar puertos conocidos
            for port in range(8001, 8011):
                worker_url = f"http://127.0.0.1:{port}"
                try:
                    # Intentar con la raíz primero
                    async with httpx.AsyncClient(timeout=1.0) as client:
                        response = await client.get(f"{worker_url}/", timeout=1.0)
                        if response.status_code < 500:  # Aceptar cualquier respuesta que no sea error del servidor
                            self.workers_urls.add(worker_url)
                            logger.info(f"Recuperado worker de emergencia: {worker_url}")
                except Exception:
                    pass
            
            # Si no se encontraron workers, intentar iniciar uno nuevo
            if not self.workers_urls and hasattr(self, 'server_adapter') and hasattr(self, 'app'):
                logger.warning("Intentando iniciar worker de emergencia")
                from proxa.process import start_worker
                worker_info = await start_worker(self.server_adapter, self.app, self.active_workers)
                if worker_info:
                    logger.info(f"Worker de emergencia iniciado en puerto {worker_info.port}")
                    self._sync_workers()  # Sincronizar para incluir el nuevo worker
        except Exception as e:
            logger.error(f"Error en recuperación de emergencia: {e}")
    
    async def forward_request(self, request_data: bytes, worker_url: str) -> bytes:
        """Reenvía una solicitud HTTP a un worker y devuelve la respuesta."""
        try:
            # Extraer la primera línea para obtener el método y la ruta
            first_line = request_data.split(b'\r\n')[0].decode('utf-8')
            method, path, _ = first_line.split(' ')
            
            # Construir URL completa
            url = f"{worker_url}{path}"
            
            # Extraer headers
            headers = {}
            header_lines = request_data.split(b'\r\n\r\n')[0].split(b'\r\n')[1:]
            for line in header_lines:
                if not line:
                    continue
                try:
                    key, value = line.decode('utf-8').split(':', 1)
                    headers[key.strip()] = value.strip()
                except Exception as e:
                    logger.warning(f"Error al parsear header: {line}: {e}")
            
            # Extraer cuerpo de la solicitud
            body = None
            if b'\r\n\r\n' in request_data:
                body = request_data.split(b'\r\n\r\n', 1)[1]
                if not body:
                    body = None
            
            # Realizar solicitud al worker con reintentos y timeouts más cortos
            max_retries = 2
            for retry in range(max_retries):
                try:
                    # Usar timeouts más cortos para evitar bloqueos
                    timeout = httpx.Timeout(5.0, connect=2.0)
                    async with httpx.AsyncClient(timeout=timeout) as client:
                        response = await client.request(
                            method=method,
                            url=url,
                            headers=headers,
                            content=body
                        )
                        
                        # Construir respuesta HTTP completa
                        status_line = f"HTTP/1.1 {response.status_code} {response.reason_phrase}\r\n"
                        headers_lines = '\r\n'.join([f"{k}: {v}" for k, v in response.headers.items()])
                        
                        response_data = f"{status_line}{headers_lines}\r\n\r\n".encode('utf-8') + response.content
                        return response_data
                except Exception as e:
                    logger.warning(f"Error en intento {retry+1}/{max_retries} al reenviar a {url}: {e}")
                    if retry == max_retries - 1:
                        raise
                    await anyio.sleep(0.1)  # Esperar menos tiempo entre reintentos
                    
        except Exception as e:
            logger.error(f"Error al reenviar solicitud a {worker_url}: {e}")
            error_msg = f"Error al comunicarse con el worker: {str(e)}"
            response = f"HTTP/1.1 502 Bad Gateway\r\nContent-Length: {len(error_msg)}\r\n\r\n{error_msg}"
            return response.encode('utf-8')