import anyio
import logging
from typing import Dict, Any

from proxa.process import start_worker, stop_worker, WorkerInfo
from proxa.metrics import monitor_workers
from proxa.health import run_health_server
from proxa.servers import get_server_adapter
from proxa.types import ProxaConfig
from proxa.proxy import ProxyServer
import msgspec

logger = logging.getLogger(__name__)

# Global dictionary to track active workers
active_workers: Dict[int, WorkerInfo] = {}

async def run_proxa(config: Dict[str, Any]):
    """
    Main function that runs Proxa with the provided configuration.
    """
    logger.info(f"Starting Proxa with server: {config.server}, app: {config.app}")
    logger.info(f"Initial workers: {config.workers} (Min: {config.min_workers}, "
                f"Max: {config.max_workers if config.max_workers else 'Unlimited'})")
    
    # Get the appropriate server adapter
    server_adapter = get_server_adapter(config.server)
    
    try:
        async with anyio.create_task_group() as tg:
            # Start proxy server
            proxy = ProxyServer("127.0.0.1", 8000, active_workers)
            tg.start_soon(proxy.start)
            
            # Start initial workers
            for _ in range(config.workers):
                worker = await start_worker(server_adapter, config.app, active_workers)
                if not worker:
                    logger.error("Could not start initial worker. Aborting.")
                    return
            
            # Start monitoring task - Corregido para usar start_soon en lugar de start con argumentos con nombre
            tg.start_soon(monitor_workers, 
                          active_workers,
                          config.min_workers,
                          config.max_workers,
                          config.max_cpu,
                          config.max_mem,
                          server_adapter,
                          config.app)
            
            # If enabled, start the health endpoint server
            if config.enable_health:
                logger.info(f"Starting health endpoint at http://{config.health_host}:{config.health_port}")
                tg.start_soon(run_health_server, 
                              config.health_host, 
                              config.health_port,
                              active_workers)
            
            # The task group will keep running until a task fails
            # or is cancelled (e.g., with Ctrl+C)
            logger.info("Proxa started and monitoring. Press Ctrl+C to stop.")
            
    except Exception as e:
        logger.error(f"Unexpected error in main loop: {e}", exc_info=True)
    finally:
        logger.info("Starting orderly shutdown...")
        # Stop all active workers on exit
        async with anyio.create_task_group() as shutdown_tg:
            for pid in list(active_workers.keys()):
                shutdown_tg.start_soon(stop_worker, pid, active_workers)
        logger.info("Workers stopped.")
        logger.info("Proxa finished.")


class ProxaCore:
    def __init__(self, config_path: str = None):
        self.config = ProxaConfig(
            host="127.0.0.1",
            port=8000,
            workers=2,
            server_type="uvicorn"
        )
        
        if config_path:
            self.load_config(config_path)
    
    def load_config(self, config_path: str):
        with open(config_path, 'rb') as f:
            data = f.read()
            self.config = msgspec.json.decode(data, type=ProxaConfig)
    
    def save_config(self, config_path: str):
        data = msgspec.json.encode(self.config)
        with open(config_path, 'wb') as f:
            f.write(data)