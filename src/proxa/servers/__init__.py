from typing import Any

from proxa.servers.base import ServerAdapter
from proxa.servers.uvicorn import UvicornAdapter
from proxa.servers.hypercorn import HypercornAdapter
from proxa.servers.granian import GranianAdapter

def get_server_adapter(server_name: str) -> ServerAdapter:
    """
    Devuelve el adaptador de servidor adecuado según el nombre.
    """
    server_name = server_name.lower()
    
    if server_name == 'uvicorn':
        return UvicornAdapter()
    elif server_name == 'hypercorn':
        return HypercornAdapter()
    elif server_name == 'granian':
        return GranianAdapter()
    else:
        raise ValueError(f"Servidor no soportado: {server_name}")