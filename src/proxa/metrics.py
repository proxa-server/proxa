import anyio
import logging
import time
import psutil
from typing import Dict, Optional, Any
from proxa.types import WorkerStats, WorkerMetrics

from proxa.process import WorkerInfo, start_worker, stop_worker

logger = logging.getLogger(__name__)

class MetricsCollector:
    def __init__(self):
        self.stats: Dict[str, WorkerStats] = {}
    
    def update_worker_stats(self, pid: int, cpu: float, memory: float):
        stats = WorkerStats(
            pid=pid,
            cpu_percent=cpu,
            memory_mb=memory,
            last_update=time.time()
        )
        self.stats[str(pid)] = stats
    
    def get_metrics(self) -> WorkerMetrics:
        return WorkerMetrics(
            total_workers=len(self.stats),
            active_workers=len([s for s in self.stats.values() if s.last_update > time.time() - 30]),
            stats=self.stats
        )

async def update_worker_metrics(active_workers: Dict[int, WorkerInfo]):
    """
    Updates metrics for all active workers.
    """
    current_time = time.time()
    for pid, worker_info in list(active_workers.items()):
        try:
            if worker_info.psutil_process:
                # Update CPU usage (as percentage)
                worker_info.cpu_percent = worker_info.psutil_process.cpu_percent(interval=None)
                
                # Update memory usage (in MB)
                memory_info = worker_info.psutil_process.memory_info()
                worker_info.memory_mb = memory_info.rss / (1024 * 1024)  # Convert bytes to MB
                
                # Update timestamp
                worker_info.last_update = current_time
                
                logger.debug(f"Worker {pid} metrics - CPU: {worker_info.cpu_percent:.1f}%, "
                           f"Memory: {worker_info.memory_mb:.1f} MB")
        except psutil.NoSuchProcess:
            logger.warning(f"Process {pid} no longer exists, removing from active workers")
            del active_workers[pid]
        except Exception as e:
            logger.error(f"Error updating metrics for worker {pid}: {e}")

async def check_scaling(
    active_workers: Dict[int, WorkerInfo],
    min_workers: int,
    max_workers: Optional[int],
    max_cpu: float,
    max_mem: Optional[float],
    server_adapter: Any,
    app: str
):
    """
    Checks if scaling up or down is needed based on current metrics.
    """
    # Calculate average CPU usage across all workers
    if not active_workers:
        return
    
    total_cpu = sum(w.cpu_percent for w in active_workers.values() if hasattr(w, 'cpu_percent'))
    avg_cpu = total_cpu / len(active_workers)
    
    # Check if we need to scale up (high CPU usage)
    if avg_cpu > max_cpu:
        # Only scale up if we haven't reached max_workers
        if max_workers is None or len(active_workers) < max_workers:
            logger.info(f"Scaling up due to high CPU usage ({avg_cpu:.1f}% > {max_cpu:.1f}%)")
            await start_worker(server_adapter, app, active_workers)
    
    # Check if we need to scale down (low CPU usage)
    elif avg_cpu < (max_cpu * 0.5) and len(active_workers) > min_workers:
        # Find the worker with the lowest CPU usage
        lowest_cpu_pid = min(active_workers.items(), key=lambda x: x[1].cpu_percent if hasattr(x[1], 'cpu_percent') else float('inf'))[0]
        logger.info(f"Scaling down due to low CPU usage ({avg_cpu:.1f}% < {max_cpu * 0.5:.1f}%)")
        await stop_worker(lowest_cpu_pid, active_workers)
    
    # Check memory limits if specified
    if max_mem is not None:
        for pid, worker in list(active_workers.items()):
            if hasattr(worker, 'memory_mb') and worker.memory_mb > max_mem:
                logger.warning(f"Worker {pid} exceeded memory limit ({worker.memory_mb:.1f} MB > {max_mem:.1f} MB)")
                # Restart the worker
                await stop_worker(pid, active_workers)
                await start_worker(server_adapter, app, active_workers)

async def monitor_workers(
    active_workers: Dict[int, WorkerInfo],
    min_workers: int,
    max_workers: Optional[int],
    max_cpu: float,
    max_mem: Optional[float],
    server_adapter: Any,
    app: str,
    task_status=anyio.TASK_STATUS_IGNORED
):
    """
    Monitors worker processes and scales up/down based on metrics.
    """
    try:
        # Signal that the task has started
        if hasattr(task_status, 'started'):
            task_status.started()
        
        logger.info("Worker monitoring started")
        
        while True:
            # Update metrics for all workers
            await update_worker_metrics(active_workers)
            
            # Check if we need to scale up or down
            await check_scaling(
                active_workers, 
                min_workers, 
                max_workers, 
                max_cpu, 
                max_mem,
                server_adapter,
                app
            )
            
            # Sleep before next check
            await anyio.sleep(5)  # Check every 5 seconds
            
    except Exception as e:
        logger.error(f"Error in worker monitoring: {e}", exc_info=True)