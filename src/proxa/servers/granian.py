import sys
import anyio
import logging
from proxa.servers.base import ServerAdapter

logger = logging.getLogger(__name__)

class GranianAdapter(ServerAdapter):
    """Adapter for the Granian server."""
    
    @property
    def name(self) -> str:
        return "granian"
    
    async def start_worker(self, app: str, port: int) -> anyio.abc.Process:
        """
        Starts a Granian worker for the specified application.
        
        Args:
            app: ASGI application import path
            port: Port where the worker will listen
        """
        # Construir comando para Granian
        command = [
            sys.executable, 
            "-m", 
            "granian", 
            app,
            "--interface", "asgi",
            "--host", "127.0.0.1",
            "--port", str(port),
            "--log-level=info"
        ]
        
        logger.info(f"Iniciando worker de Granian con comando: {' '.join(command)}")
        
        process = await anyio.open_process(
            command, 
            stdin=None,
            stdout=None,
            stderr=None
        )
        
        return process