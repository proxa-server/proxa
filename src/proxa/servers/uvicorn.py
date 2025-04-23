import sys
import anyio
import logging
from proxa.servers.base import ServerAdapter

logger = logging.getLogger(__name__)

class UvicornAdapter(ServerAdapter):
    """Adapter for the Uvicorn server."""
    
    @property
    def name(self) -> str:
        return "uvicorn"
    
    async def start_worker(self, app: str, port: int) -> anyio.abc.Process:
        """
        Inicia un worker de Uvicorn para la aplicación especificada.
        """
        command = [
            sys.executable, 
            "-m", 
            "uvicorn", 
            app,
            "--host", "127.0.0.1",
            "--port", str(port),
            "--log-level=info"
        ]
        
        logger.info(f"Iniciando worker de Uvicorn con comando: {' '.join(command)}")
        
        process = await anyio.open_process(
            command, 
            stdin=None,
            stdout=None,
            stderr=None
        )
        
        return process