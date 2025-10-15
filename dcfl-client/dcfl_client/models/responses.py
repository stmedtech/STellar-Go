"""Response data models."""

from typing import List, Dict, Any, Optional, Callable
from datetime import datetime
from pydantic import BaseModel, Field, ConfigDict
from pydantic.alias_generators import to_snake


class FileEntry(BaseModel):
    """File entry model."""
    model_config = ConfigDict(alias_generator=to_snake, populate_by_name=True)

    directory_name: str = Field(alias="DirectoryName", default="")
    filename: str = Field(alias="Filename", default="")
    size: int = Field(alias="Size", default=0)
    is_dir: bool = Field(alias="IsDir", default=False)
    children: Optional[List["FileEntry"]] = Field(alias="Children", default_factory=list)

    def full_name(self) -> str:
        """Get full file path."""
        if self.directory_name:
            return f"{self.directory_name}/{self.filename}".replace("//", "/")
        return self.filename


class FileTree(BaseModel):
    """File tree model."""
    model_config = ConfigDict(alias_generator=to_snake, populate_by_name=True)

    files: List[FileEntry]


class ExecutionResult(BaseModel):
    """Script execution result model."""
    model_config = ConfigDict(alias_generator=to_snake, populate_by_name=True)

    success: bool
    output: str = ""
    result: str = ""
    env: str = ""
    exit_code: int = 0
    error: Optional[str] = None
    execution_time: Optional[float] = None


class ProxyInfo(BaseModel):
    """Proxy connection information."""
    model_config = ConfigDict(alias_generator=to_snake, populate_by_name=True)

    local_port: int = Field(alias="Port")
    remote_addr: str = Field(alias="DestAddr")
    device_id: str = Field(alias="Dest")


class CondaEnvConfig(BaseModel):
    """Conda environment configuration."""
    model_config = ConfigDict(alias_generator=to_snake, populate_by_name=True)

    name: str
    python_version: str = "3.9"
    packages: List[str] = []
    pip_packages: List[str] = []
    conda_file: Optional[str] = None
    env: str = "base"  # For backward compatibility
    version: str = "1.0"  # For backward compatibility
    env_yaml_path: str = ""  # For backward compatibility


class ScriptConfig(BaseModel):
    """Script execution configuration."""
    model_config = ConfigDict(alias_generator=to_snake, populate_by_name=True)

    script_path: str
    env: str = "base"
    python_version: str = "3.9"
    conda_env: Optional[str] = None
    args: List[str] = []
    env_vars: Dict[str, str] = {}
    timeout: int = 30
    working_dir: Optional[str] = None


class FLTaskConfig(BaseModel):
    """Federated learning task configuration."""
    model_config = ConfigDict(alias_generator=to_snake, populate_by_name=True)

    client_script: str
    framework: str = "flower"  # flower, nvflare
    python_version: str = "3.9"
    conda_env: Optional[str] = None
    timeout: int = 300
    server_config: Optional[Dict[str, Any]] = None
    rounds: int = 10
    clients_per_round: int = 2
    round_num: int = 0


class FLResult(BaseModel):
    """Federated learning result."""
    model_config = ConfigDict(alias_generator=to_snake, populate_by_name=True)

    success: bool
    model_update: Optional[str] = None
    metrics: Dict[str, Any] = {}
    device_id: str = ""
    rounds_completed: int = 0
    final_metrics: Dict[str, Any] = {}
    client_results: List[Dict[str, Any]] = []
    error: Optional[str] = None
