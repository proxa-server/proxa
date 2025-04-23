import os
import logging
import msgspec
from typing import Dict, Any, Optional, Union

logger = logging.getLogger(__name__)

# Importación dinámica del módulo TOML
try:
    # Intentar importar tomllib (Python 3.11+)
    import tomllib
except ImportError:
    try:
        # Fallback a tomli para versiones anteriores
        import tomli as tomllib
    except ImportError:
        logger.error("No se encontró ningún módulo TOML. Instala 'tomli' para Python < 3.11")
        raise
from typing import Optional, Dict, Any
from dataclasses import dataclass, asdict

@dataclass
class ProxaConfig:
    """Configuración para Proxa."""
    server: str = "uvicorn"
    app: str = ""
    workers: int = 1
    min_workers: int = 1
    max_workers: Optional[int] = None
    max_cpu: float = 80.0
    max_mem: Optional[float] = None
    enable_health: bool = True
    health_host: str = "127.0.0.1"
    health_port: int = 9000

    def to_dict(self) -> Dict[str, Any]:
        """Convierte la configuración a un diccionario."""
        return asdict(self)

    @classmethod
    def from_dict(cls, config_dict: Dict[str, Any]) -> 'ProxaConfig':
        """Crea una instancia de configuración desde un diccionario."""
        return cls(**{
            k: v for k, v in config_dict.items() 
            if k in cls.__dataclass_fields__
        })
    
    # Validación adicional
    def __post_init__(self):
        if self.min_workers < 1:
            raise ValueError("min_workers debe ser al menos 1")
        if self.workers < self.min_workers:
            raise ValueError(f"workers ({self.workers}) debe ser >= min_workers ({self.min_workers})")
        if self.max_workers is not None and self.max_workers < self.workers:
            raise ValueError(f"max_workers ({self.max_workers}) debe ser >= workers ({self.workers})")

def load_config(config_path: str) -> Dict[str, Any]:
    """Carga la configuración desde un archivo .toml o .conf."""
    if not os.path.exists(config_path):
        raise FileNotFoundError(f"Archivo de configuración no encontrado: {config_path}")
    
    _, ext = os.path.splitext(config_path)
    config_dict = {}
    
    try:
        if ext.lower() == '.toml':
            with open(config_path, 'rb') as f:
                config_dict = tomli.load(f)
        elif ext.lower() in ('.conf', '.ini'):
            # Implementar parser para archivos .conf/.ini si es necesario
            # Podría usar configparser de la stdlib
            logger.warning("Soporte para archivos .conf/.ini aún no implementado")
        else:
            logger.warning(f"Formato de configuración no soportado: {ext}")
    except Exception as e:
        logger.error(f"Error al cargar configuración desde {config_path}: {e}")
        raise
    
    return config_dict

def merge_config(file_config: Dict[str, Any], cli_config: Dict[str, Any]) -> Dict[str, Any]:
    """
    Fusiona configuraciones de archivo y línea de comandos.
    La configuración de línea de comandos tiene prioridad.
    """
    merged = file_config.copy()
    
    # Solo sobrescribir valores no-None de la CLI
    for key, value in cli_config.items():
        if value is not None or key not in merged:
            merged[key] = value
    
    # Validar configuración final
    try:
        return msgspec.convert(merged, type=ProxaConfig)
    except Exception as e:
        logger.error(f"Error en la configuración: {e}")
        raise