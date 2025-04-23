import msgspec
import time
import random
from typing import List, Dict, Optional, Any
from typing import Dict, List, Optional

class WorkerMetrics(msgspec.Struct):
    """Metrics for a single worker."""
    pid: int
    cpu_percent: Optional[float] = None
    memory_mb: Optional[float] = None
    last_update: Optional[float] = None

class WorkerStats(msgspec.Struct):
    """Statistics for a worker process."""
    pid: int
    cpu_percent: float | None
    memory_mb: float | None
    last_update: float | None
    requests: int = 0
    errors: int = 0

class HealthResponse(msgspec.Struct):
    """Response for the health endpoint."""
    status: str
    workers: int
    timestamp: float

class ErrorResponse(msgspec.Struct):
    """Response for error cases."""
    error: str
    available_endpoints: List[str]

class ProxyStats(msgspec.Struct):
    """Statistics for a proxy worker."""
    requests: int = 0
    errors: int = 0
    last_used: float = 0.0
    cpu_usage: float = 0.0
    memory_usage: float = 0.0

class ProxyConfig(msgspec.Struct):
    """Configuration for the proxy."""
    host: str
    port: int
    workers: List[str]
    worker_stats: Dict[str, ProxyStats]

class ProxaConfig(msgspec.Struct):
    """Configuration for Proxa application."""
    host: str
    port: int
    workers: int
    server_type: str
    min_workers: int = 1
    max_workers: Optional[int] = None
    max_cpu: float = 80.0
    max_mem: float = 80.0
    enable_health: bool = False
    health_host: str = "127.0.0.1"
    health_port: int = 9000

class RoutingStats(msgspec.Struct):
    """Estadísticas para un worker en la tabla de enrutamiento."""
    requests: int = 0
    errors: int = 0
    last_used: float = 0.0
    cpu_usage: float = 0.0
    memory_usage: float = 0.0
    response_time: float = 0.0  # Tiempo de respuesta promedio
    success_rate: float = 1.0   # Tasa de éxito (0.0-1.0)
    weight: float = 1.0         # Peso para selección ponderada

class RoutingTable(msgspec.Struct):
    """Tabla de enrutamiento para seleccionar workers de manera eficiente."""
    workers: List[str] = msgspec.field(default_factory=list)
    worker_stats: Dict[str, RoutingStats] = msgspec.field(default_factory=dict)
    last_index: int = -1
    
    def add_worker(self, worker_url: str) -> None:
        """Añade un worker a la tabla de enrutamiento."""
        if worker_url not in self.workers:
            self.workers.append(worker_url)
            self.worker_stats[worker_url] = RoutingStats(
                requests=0,
                errors=0,
                last_used=time.time(),
                cpu_usage=0.0,
                memory_usage=0.0,
                response_time=0.0,
                success_rate=1.0,
                weight=1.0
            )
    
    def remove_worker(self, worker_url: str) -> None:
        """Elimina un worker de la tabla de enrutamiento."""
        if worker_url in self.workers:
            self.workers.remove(worker_url)
            if worker_url in self.worker_stats:
                del self.worker_stats[worker_url]
    
    def update_stats(self, worker_url: str, **kwargs: Any) -> None:
        """Actualiza las estadísticas de un worker."""
        if worker_url in self.worker_stats:
            stats = self.worker_stats[worker_url]
            for key, value in kwargs.items():
                if hasattr(stats, key):
                    setattr(stats, key, value)
    
    def select_worker_round_robin(self) -> Optional[str]:
        """Selecciona un worker usando round-robin."""
        if not self.workers:
            return None
        
        self.last_index = (self.last_index + 1) % len(self.workers)
        selected = self.workers[self.last_index]
        
        # Actualizar estadísticas
        if selected in self.worker_stats:
            self.worker_stats[selected].last_used = time.time()
        
        return selected
    
    def select_worker_weighted(self) -> Optional[str]:
        """Selecciona un worker usando selección ponderada basada en carga y rendimiento."""
        if not self.workers:
            return None
        
        # Calcular pesos basados en CPU, tasa de éxito y tiempo de respuesta
        weights = []
        for worker in self.workers:
            stats = self.worker_stats[worker]
            
            # Invertir CPU para que menor CPU = mayor peso
            cpu_factor = 1.0 - (stats.cpu_usage / 100.0) if stats.cpu_usage <= 100 else 0.0
            
            # Factores de rendimiento
            success_factor = stats.success_rate
            response_factor = 1.0 / (1.0 + stats.response_time) if stats.response_time > 0 else 1.0
            
            # Peso combinado (ajustar estos factores según necesidad)
            weight = (0.5 * cpu_factor) + (0.3 * success_factor) + (0.2 * response_factor)
            weights.append(max(0.1, weight))  # Asegurar un peso mínimo
        
        # Selección ponderada
        total = sum(weights)
        if total <= 0:
            return self.select_worker_round_robin()  # Fallback a round-robin
        
        # Normalizar pesos
        normalized_weights = [w/total for w in weights]
        
        # Selección aleatoria ponderada
        r = random.random()
        cumulative = 0
        for i, weight in enumerate(normalized_weights):
            cumulative += weight
            if r <= cumulative:
                selected = self.workers[i]
                
                # Actualizar estadísticas
                if selected in self.worker_stats:
                    self.worker_stats[selected].last_used = time.time()
                
                return selected
        
        # Fallback (no debería llegar aquí)
        return self.select_worker_round_robin()
    
    def select_worker(self, strategy: str = "weighted") -> Optional[str]:
        """Selecciona un worker usando la estrategia especificada."""
        if strategy == "round_robin":
            return self.select_worker_round_robin()
        elif strategy == "weighted":
            return self.select_worker_weighted()
        else:
            return self.select_worker_weighted()