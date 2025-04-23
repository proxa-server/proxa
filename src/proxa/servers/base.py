from abc import ABC, abstractmethod
import anyio
from typing import Any, Optional
import msgspec

class ServerConfig(msgspec.Struct):
    """Configuración base para servidores ASGI."""
    host: str
    port: int
    workers: int
    log_level: str = "info"
    reload: bool = False
    timeout: int = 30

class ServerAdapter(ABC):
    """Clase base abstracta para adaptadores de servidor."""
    
    def __init__(self):
        self.config: ServerConfig | None = None
    
    @property
    @abstractmethod
    def name(self) -> str:
        """Retorna el nombre del servidor."""
        pass
    
    @abstractmethod
    async def start_worker(self, app: str, port: int) -> anyio.abc.Process:
        """
        Inicia un nuevo worker para la aplicación especificada.
        
        Args:
            app: La aplicación en formato "module:app"
            port: El puerto en el que el worker debe escuchar
            
        Returns:
            Un objeto Process de anyio que representa el worker iniciado
        """
        pass