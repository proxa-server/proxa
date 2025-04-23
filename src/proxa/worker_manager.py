import anyio
import logging
import random
from typing import Dict, List, Optional, Set
import psutil
from proxa.servers.base import ServerAdapter
from proxa.proxy import ProxaProxy
import msgspec
from typing import Dict, Set

logger = logging.getLogger(__name__)

class WorkerManager:
    """
    Worker manager for Proxa.
    Handles worker creation, monitoring and termination.
    """
    def __init__(self, 
                 proxy: ProxaProxy,
                 min_port: int = 8001,
                 max_port: int = 8999):
        self.proxy = proxy
        self.min_port = min_port
        self.max_port = max_port
        self.used_ports: Set[int] = set()
        self.workers: Dict[str, Dict] = {}
        
    def get_available_port(self) -> Optional[int]:
        """Gets an available port for a new worker."""
        available_ports = set(range(self.min_port, self.max_port + 1)) - self.used_ports
        if not available_ports:
            return None
        port = random.choice(list(available_ports))
        self.used_ports.add(port)
        return port
    
    def release_port(self, port: int):
        """Releases a port so it can be reused."""
        if port in self.used_ports:
            self.used_ports.remove(port)
    
    async def start_worker(self, 
                          adapter: ServerAdapter,
                          app: str) -> Optional[Dict]:
        """
        Starts a new worker.
        
        Args:
            adapter: Server adapter to use
            app: ASGI application import path
        """
        port = self.get_available_port()
        if not port:
            logger.error("No ports available for new workers")
            return None
            
        try:
            # Start worker process
            process = await adapter.start_worker(app, port)
            if not process:
                self.release_port(port)
                return None
                
            # Build worker URL
            worker_url = f"http://127.0.0.1:{port}"
            
            # Register worker
            worker_info = {
                "process": process,
                "port": port,
                "url": worker_url,
                "adapter": adapter,
                "app": app
            }
            self.workers[worker_url] = worker_info
            
            # Add to proxy
            self.proxy.add_worker(worker_url)
            
            logger.info(f"Worker started successfully at {worker_url}")
            return worker_info
            
        except Exception as e:
            logger.error(f"Error starting worker: {e}")
            self.release_port(port)
            return None
    
    async def stop_worker(self, worker_url: str):
        """
        Detiene un worker específico.
        """
        if worker_url not in self.workers:
            return
            
        worker_info = self.workers[worker_url]
        try:
            # Eliminar del proxy
            self.proxy.remove_worker(worker_url)
            
            # Detener proceso
            process = worker_info["process"]
            process.terminate()
            await process.wait()
            
            # Liberar puerto
            self.release_port(worker_info["port"])
            
            # Eliminar registro
            del self.workers[worker_url]
            
            logger.info(f"Worker detenido exitosamente: {worker_url}")
            
        except Exception as e:
            logger.error(f"Error al detener worker {worker_url}: {e}")
    
    async def monitor_workers(self):
        """
        Monitorea el estado de los workers y actualiza sus estadísticas.
        """
        while True:
            for worker_url, worker_info in list(self.workers.items()):
                try:
                    # Verificar si el proceso sigue vivo
                    process = worker_info["process"]
                    if process.returncode is not None:
                        logger.warning(f"Worker {worker_url} ha terminado inesperadamente")
                        await self.stop_worker(worker_url)
                        continue
                    
                    # Obtener estadísticas del proceso
                    proc = psutil.Process(process.pid)
                    cpu_percent = proc.cpu_percent()
                    memory_percent = proc.memory_percent()
                    
                    # Actualizar estadísticas en el proxy
                    if worker_url in self.proxy.config.worker_stats:
                        stats = self.proxy.config.worker_stats[worker_url]
                        stats.cpu_usage = cpu_percent
                        stats.memory_usage = memory_percent
                    
                except Exception as e:
                    logger.error(f"Error monitoreando worker {worker_url}: {e}")
                    await self.stop_worker(worker_url)
            
            await anyio.sleep(5)  # Verificar cada 5 segundos
    
    async def scale_workers(self, 
                          adapter: ServerAdapter,
                          app: str,
                          target_workers: int):
        """
        Escala el número de workers al objetivo especificado.
        
        Args:
            adapter: Adaptador del servidor a utilizar
            app: Ruta de importación de la aplicación ASGI
            target_workers: Número objetivo de workers
        """
        current_count = len(self.workers)
        
        if current_count < target_workers:
            # Necesitamos más workers
            for _ in range(target_workers - current_count):
                await self.start_worker(adapter, app)
        elif current_count > target_workers:
            # Necesitamos menos workers
            workers_to_remove = list(self.workers.keys())[:(current_count - target_workers)]
            for worker_url in workers_to_remove:
                await self.stop_worker(worker_url)


class WorkerState(msgspec.Struct):
    """Estado de un worker individual."""
    pid: int
    port: int
    url: str
    active: bool
    last_heartbeat: float
    process: Any  # No podemos tipar Process directamente en msgspec

class WorkerManagerState(msgspec.Struct):
    """Estado global del gestor de workers."""
    used_ports: Set[int]
    workers: Dict[str, WorkerState]
    total_workers: int
    active_workers: int