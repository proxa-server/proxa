import anyio
import logging
import psutil
from typing import Dict, Optional, Any
import random

logger = logging.getLogger(__name__)

class WorkerInfo:
    """Class to maintain information about a worker."""
    def __init__(self, process: anyio.abc.Process, pid: int, port: int):
        self.process = process
        self.pid = pid
        self.port = port
        self.psutil_process: Optional[psutil.Process] = None
        try:
            self.psutil_process = psutil.Process(pid)
        except psutil.NoSuchProcess:
            logger.warning(f"Could not find psutil process for PID: {pid}")
        except psutil.AccessDenied:
            logger.warning(f"Access denied to get psutil process information for PID: {pid}")
        
        # Metrics
        self.cpu_percent: float = 0.0
        self.memory_mb: float = 0.0
        self.last_update: float = 0.0  # timestamp

async def start_worker(server_adapter: Any, app: str, active_workers: Dict[int, WorkerInfo], base_port: int = 8001) -> Optional[WorkerInfo]:
    """
    Starts a new worker for the specified application.
    
    Args:
        server_adapter: The server adapter to use
        app: The application in "module:app" format
        active_workers: Dictionary of active workers
        base_port: Base port for workers (default 8001)
    """
    try:
        # Generate port for the worker
        max_port = base_port + 998  # Limit to 999 workers maximum
        port = base_port
        used_ports = {worker.port for worker in active_workers.values()}
        
        while port <= max_port:
            if port not in used_ports:
                break
            port += 1
        else:
            logger.error("No ports available for new workers")
            return None
            
        # The server adapter handles command construction
        process = await server_adapter.start_worker(app, port)
        if not process:
            logger.error("Server adapter failed to start worker")
            return None
            
        worker_info = WorkerInfo(process, process.pid, port)
        active_workers[process.pid] = worker_info
        logger.info(f"Worker started with PID: {process.pid} on port {port}")
        return worker_info
    except Exception as e:
        logger.error(f"Error starting worker: {e}")
        return None

async def stop_worker(pid: int, active_workers: Dict[int, WorkerInfo], signal_num: int = 15):
    """Sends a signal to a worker to stop it."""
    if pid in active_workers:
        worker_info = active_workers[pid]
        logger.info(f"Sending signal {signal_num} to worker with PID: {pid}")
        try:
            # First try with psutil if available
            if worker_info.psutil_process and worker_info.psutil_process.is_running():
                try:
                    worker_info.psutil_process.send_signal(signal_num)
                    logger.info(f"Signal sent using psutil to worker {pid}")
                    
                    # Wait for termination
                    worker_info.psutil_process.wait(timeout=5)
                except psutil.TimeoutExpired:
                    logger.warning(f"Worker {pid} did not respond to SIGTERM, sending SIGKILL")
                    worker_info.psutil_process.kill()
                    logger.info(f"Worker {pid} terminated with SIGKILL")
            else:
                logger.warning(f"Could not find process {pid} using psutil")
                
        except Exception as e:
            logger.error(f"Error stopping worker {pid}: {e}")
        finally:
            # Remove from active list
            if pid in active_workers:
                del active_workers[pid]
    else:
        logger.warning(f"Attempt to stop worker with PID {pid} not found.")