import sys
import anyio
import logging
from proxa.servers.base import ServerAdapter

logger = logging.getLogger(__name__)

class HypercornAdapter(ServerAdapter):
    """Adapter for the Hypercorn server."""
    
    @property
    def name(self) -> str:
        return "hypercorn"
    
    async def start_worker(self, app: str, port: int) -> anyio.abc.Process:
        """
        Inicia un worker de Hypercorn para la aplicación especificada.
        """
        # Construir el comando para Hypercorn
        command = [
            sys.executable, 
            "-m", 
            "hypercorn", 
            app,
            "--bind", f"127.0.0.1:{port}",
            "--log-level", "info"
        ]
        
        logger.info(f"Iniciando worker de Hypercorn con comando: {' '.join(command)}")
        
        # Iniciar el proceso
        process = await anyio.open_process(
            command, 
            stdin=None,  # Heredar stdin
            stdout=None,  # Heredar stdout
            stderr=None   # Heredar stderr
        )
        
        return process