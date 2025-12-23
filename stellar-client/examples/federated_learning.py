#!/usr/bin/env python3
"""
Federated Learning Example

This example demonstrates a complete federated learning workflow using Stellar:
- Setting up FL environments on multiple devices
- Deploying FL client scripts
- Coordinating federated training
- Collecting and aggregating results
"""

import tempfile
import os
import json
import time
from stellar_client import StellarClient
from stellar_client.models import CondaEnvConfig, ScriptConfig, FLTaskConfig
from stellar_client.exceptions import ComputeError, DeviceNotFoundError


def create_fl_environment_yaml() -> str:
    """Create environment.yml for federated learning setup."""
    env_content = """name: fl_env
channels:
  - defaults
  - conda-forge
dependencies:
  - python=3.9
  - numpy
  - torch
  - torchvision
  - matplotlib
  - pip
  - pip:
    - flwr[simulation]>=1.4.0
    - scikit-learn
"""
    with tempfile.NamedTemporaryFile(mode='w', delete=False, suffix='.yml') as f:
        f.write(env_content)
        return f.name


def create_fl_client_script() -> str:
    """Create a simple Flower FL client script."""
    client_script = '''
import sys
import numpy as np
import torch
import torch.nn as nn
import torch.nn.functional as F
from torch.utils.data import DataLoader, TensorDataset
from sklearn.datasets import make_classification
from sklearn.model_selection import train_test_split
import flwr as fl
from flwr.client import NumPyClient
import json

# Simple neural network model
class SimpleNet(nn.Module):
    def __init__(self, input_dim=20, hidden_dim=50, output_dim=2):
        super(SimpleNet, self).__init__()
        self.fc1 = nn.Linear(input_dim, hidden_dim)
        self.fc2 = nn.Linear(hidden_dim, hidden_dim)
        self.fc3 = nn.Linear(hidden_dim, output_dim)
        
    def forward(self, x):
        x = F.relu(self.fc1(x))
        x = F.relu(self.fc2(x))
        x = self.fc3(x)
        return x

# Generate synthetic dataset
def create_dataset(n_samples=1000, n_features=20, client_id=0):
    """Create synthetic dataset with some variation per client."""
    np.random.seed(42 + client_id)  # Different seed per client
    X, y = make_classification(
        n_samples=n_samples,
        n_features=n_features,
        n_informative=n_features//2,
        n_redundant=0,
        n_clusters_per_class=1,
        random_state=42 + client_id
    )
    
    # Add some client-specific bias
    X = X + np.random.normal(0, 0.1 * client_id, X.shape)
    
    return train_test_split(X, y, test_size=0.3, random_state=42)

# Flower client implementation
class StellarClient(NumPyClient):
    def __init__(self, model, train_loader, test_loader, client_id):
        self.model = model
        self.train_loader = train_loader
        self.test_loader = test_loader
        self.client_id = client_id
        
    def get_parameters(self, config):
        return [val.cpu().numpy() for _, val in self.model.state_dict().items()]
        
    def set_parameters(self, parameters):
        params_dict = zip(self.model.state_dict().keys(), parameters)
        state_dict = {k: torch.tensor(v) for k, v in params_dict}
        self.model.load_state_dict(state_dict, strict=True)
        
    def fit(self, parameters, config):
        self.set_parameters(parameters)
        
        # Training
        self.model.train()
        optimizer = torch.optim.Adam(self.model.parameters(), lr=0.01)
        criterion = nn.CrossEntropyLoss()
        
        epochs = config.get("epochs", 5)
        for epoch in range(epochs):
            for batch_x, batch_y in self.train_loader:
                optimizer.zero_grad()
                outputs = self.model(batch_x.float())
                loss = criterion(outputs, batch_y.long())
                loss.backward()
                optimizer.step()
        
        return self.get_parameters(config={}), len(self.train_loader.dataset), {}
        
    def evaluate(self, parameters, config):
        self.set_parameters(parameters)
        
        # Evaluation
        self.model.eval()
        correct = 0
        total = 0
        loss_total = 0
        criterion = nn.CrossEntropyLoss()
        
        with torch.no_grad():
            for batch_x, batch_y in self.test_loader:
                outputs = self.model(batch_x.float())
                loss = criterion(outputs, batch_y.long())
                loss_total += loss.item()
                _, predicted = torch.max(outputs.data, 1)
                total += batch_y.size(0)
                correct += (predicted == batch_y).sum().item()
        
        accuracy = correct / total
        avg_loss = loss_total / len(self.test_loader)
        
        print(f"Client {self.client_id} - Accuracy: {accuracy:.4f}, Loss: {avg_loss:.4f}")
        
        return avg_loss, total, {"accuracy": accuracy}

def main():
    # Get client ID from command line or environment
    client_id = int(sys.argv[1]) if len(sys.argv) > 1 else 0
    server_address = sys.argv[2] if len(sys.argv) > 2 else "127.0.0.1:8080"
    
    print(f"Starting Stellar FL Client {client_id}")
    print(f"Server address: {server_address}")
    
    # Create dataset
    X_train, X_test, y_train, y_test = create_dataset(client_id=client_id)
    
    # Create data loaders
    train_dataset = TensorDataset(torch.tensor(X_train), torch.tensor(y_train))
    test_dataset = TensorDataset(torch.tensor(X_test), torch.tensor(y_test))
    train_loader = DataLoader(train_dataset, batch_size=32, shuffle=True)
    test_loader = DataLoader(test_dataset, batch_size=32, shuffle=False)
    
    # Create model
    model = SimpleNet(input_dim=X_train.shape[1])
    
    # Create FL client
    client = StellarClient(model, train_loader, test_loader, client_id)
    
    print(f"Training data size: {len(X_train)}")
    print(f"Test data size: {len(X_test)}")
    
    # Connect to FL server
    try:
        fl.client.start_numpy_client(server_address=server_address, client=client)
        print(f"FL Client {client_id} completed successfully!")
        
        # Save final model state
        final_results = {
            "client_id": client_id,
            "training_samples": len(X_train),
            "test_samples": len(X_test),
            "final_parameters_count": sum(p.numel() for p in model.parameters()),
            "status": "completed"
        }
        
        print("Final results:", json.dumps(final_results, indent=2))
        
    except Exception as e:
        print(f"FL Client {client_id} failed: {e}")
        error_results = {
            "client_id": client_id,
            "status": "failed",
            "error": str(e)
        }
        print("Error results:", json.dumps(error_results, indent=2))
        return 1
        
    return 0

if __name__ == "__main__":
    exit(main())
'''
    
    with tempfile.NamedTemporaryFile(mode='w', delete=False, suffix='.py') as f:
        f.write(client_script)
        return f.name


def create_fl_server_script() -> str:
    """Create a simple Flower FL server script."""
    server_script = '''
import flwr as fl
from flwr.server.strategy import FedAvg
from flwr.common import Metrics
from typing import List, Tuple, Dict, Optional
import json

class StellarStrategy(FedAvg):
    def __init__(self, **kwargs):
        super().__init__(**kwargs)
        self.round_results = []
        
    def aggregate_evaluate(
        self,
        server_round: int,
        results: List[Tuple[fl.server.client_proxy.ClientProxy, fl.common.EvaluateRes]],
        failures: List[Union[Tuple[fl.server.client_proxy.ClientProxy, fl.common.EvaluateRes], BaseException]],
    ) -> Tuple[Optional[float], Dict[str, fl.common.Scalar]]:
        
        if not results:
            return None, {}
            
        # Aggregate metrics
        accuracies = [r.metrics["accuracy"] for _, r in results if "accuracy" in r.metrics]
        losses = [r.loss for _, r in results]
        
        avg_accuracy = sum(accuracies) / len(accuracies) if accuracies else 0
        avg_loss = sum(losses) / len(losses) if losses else 0
        
        print(f"Round {server_round} Results:")
        print(f"  Participants: {len(results)}")
        print(f"  Average Accuracy: {avg_accuracy:.4f}")
        print(f"  Average Loss: {avg_loss:.4f}")
        
        # Store round results
        round_result = {
            "round": server_round,
            "participants": len(results),
            "avg_accuracy": avg_accuracy,
            "avg_loss": avg_loss,
            "failures": len(failures)
        }
        self.round_results.append(round_result)
        
        return avg_loss, {"accuracy": avg_accuracy, "participants": len(results)}

def main():
    print("Starting Stellar Federated Learning Server")
    
    # Configure FL strategy
    strategy = StellarStrategy(
        fraction_fit=1.0,  # Use all available clients
        fraction_evaluate=1.0,
        min_fit_clients=2,
        min_evaluate_clients=2,
        min_available_clients=2,
    )
    
    print("Server configuration:")
    print(f"  Min clients for training: {strategy.min_fit_clients}")
    print(f"  Min clients for evaluation: {strategy.min_evaluate_clients}")
    
    try:
        # Start FL server
        fl.server.start_server(
            server_address="0.0.0.0:8080",
            config=fl.server.ServerConfig(num_rounds=5),
            strategy=strategy,
        )
        
        print("Federated Learning completed successfully!")
        
        # Print final results
        print("\\nFinal Results Summary:")
        print(json.dumps(strategy.round_results, indent=2))
        
    except Exception as e:
        print(f"FL Server failed: {e}")
        return 1
        
    return 0

if __name__ == "__main__":
    exit(main())
'''
    
    with tempfile.NamedTemporaryFile(mode='w', delete=False, suffix='.py') as f:
        f.write(server_script)
        return f.name


def main():
    print("Stellar Client - Federated Learning Example")
    print("=" * 50)
    
    try:
        with StellarClient() as client:
            print("✓ Connected to Stellar node")
            
            # Get available devices
            devices = client.list_devices()
            if len(devices) < 2:
                print(f"Found only {len(devices)} device(s). Federated learning works best with 2+ devices.")
                print("Starting additional Stellar nodes is recommended for this demo.")
                if len(devices) == 0:
                    return 1
                    
            print(f"Found {len(devices)} device(s) for FL training:")
            for i, device in enumerate(devices):
                print(f"  {i+1}. {device.id} - Status: {device.status}")
            print()
            
            # Select devices for FL (up to 3 for demo)
            fl_devices = devices[:min(3, len(devices))]
            print(f"Using {len(fl_devices)} devices for federated learning")
            print()
            
            # 1. Prepare FL environment on all devices
            print("1. Preparing FL environments...")
            env_yaml = create_fl_environment_yaml()
            
            try:
                for i, device in enumerate(fl_devices):
                    print(f"  Setting up environment on device {i+1}/{len(fl_devices)}: {device.id}")
                    
                    try:
                        # Check if FL environment already exists
                        envs = client.compute.list_conda_envs(device.id)
                        
                        if "fl_env" not in envs:
                            # Upload environment file
                            client.file.upload_file(
                                device_id=device.id,
                                local_path=env_yaml,
                                remote_path="fl_environment.yml"
                            )
                            
                            # Prepare environment
                            env_config = CondaEnvConfig(
                                env="fl_env",
                                version="3.9",
                                env_yaml_path="fl_environment.yml"
                            )
                            
                            print(f"    Creating FL environment (this may take several minutes)...")
                            env_path = client.compute.prepare_environment(device.id, env_config)
                            print(f"    ✓ Environment ready at: {env_path}")
                        else:
                            print(f"    ✓ FL environment already exists")
                            
                    except ComputeError as e:
                        print(f"    ✗ Failed to prepare environment: {e}")
                        print(f"    Continuing with base environment...")
                        
            finally:
                os.unlink(env_yaml)
            print()
            
            # 2. Deploy FL client scripts
            print("2. Deploying FL client scripts...")
            client_script_path = create_fl_client_script()
            
            try:
                for i, device in enumerate(fl_devices):
                    print(f"  Deploying to device {i+1}: {device.id}")
                    
                    success = client.file.upload_file(
                        device_id=device.id,
                        local_path=client_script_path,
                        remote_path="fl_client.py"
                    )
                    
                    if success:
                        print(f"    ✓ FL client script deployed")
                    else:
                        print(f"    ✗ Failed to deploy FL client script")
                        
            finally:
                os.unlink(client_script_path)
            print()
            
            # 3. Start FL server (on first device)
            print("3. Starting FL server...")
            server_script_path = create_fl_server_script()
            server_device = fl_devices[0]
            
            try:
                # Deploy server script
                success = client.file.upload_file(
                    device_id=server_device.id,
                    local_path=server_script_path,
                    remote_path="fl_server.py"
                )
                
                if success:
                    print(f"  ✓ FL server script deployed to {server_device.id}")
                    
                    # Start server (in background)
                    print("  Starting FL server...")
                    server_config = ScriptConfig(
                        env="fl_env" if "fl_env" in client.compute.list_conda_envs(server_device.id) else "base",
                        script_path="fl_server.py"
                    )
                    
                    # Note: In a real implementation, this would be started in background
                    # For demo purposes, we'll simulate the server start
                    print("  ✓ FL server would be started here (background process)")
                    
                else:
                    print("  ✗ Failed to deploy FL server script")
                    
            finally:
                os.unlink(server_script_path)
            print()
            
            # 4. Start FL clients
            print("4. Starting FL clients...")
            client_results = []
            
            for i, device in enumerate(fl_devices):
                print(f"  Starting FL client {i} on device: {device.id}")
                
                try:
                    # Determine environment
                    envs = client.compute.list_conda_envs(device.id)
                    env_name = "fl_env" if "fl_env" in envs else "base"
                    
                    # Create client execution script that includes parameters
                    client_exec_script = f'''
# FL Client Execution Wrapper
import subprocess
import sys
import os

# Set client parameters
client_id = {i}
server_address = "127.0.0.1:8080"  # In real setup, use server device IP

print(f"Executing FL client {{client_id}}")
print(f"Server address: {{server_address}}")

# For demo purposes, simulate FL client execution
print("FL Client simulation results:")
print(f"Client ID: {{client_id}}")
print(f"Training data: 700 samples")
print(f"Test data: 300 samples") 
print("Round 1 - Accuracy: 0.7234, Loss: 0.8765")
print("Round 2 - Accuracy: 0.7891, Loss: 0.7234")
print("Round 3 - Accuracy: 0.8123, Loss: 0.6543")
print("Round 4 - Accuracy: 0.8456, Loss: 0.5987")
print("Round 5 - Accuracy: 0.8678, Loss: 0.5432")
print("FL training completed successfully!")

# Simulate final results
import json
results = {{
    "client_id": client_id,
    "final_accuracy": 0.8678 + (client_id * 0.01),  # Slight variation
    "rounds_completed": 5,
    "status": "completed"
}}

print("Final results:", json.dumps(results, indent=2))
'''
                    
                    # Create and upload client execution script
                    with tempfile.NamedTemporaryFile(mode='w', delete=False, suffix='.py') as f:
                        f.write(client_exec_script)
                        exec_script_path = f.name
                        
                    try:
                        client.file.upload_file(
                            device_id=device.id,
                            local_path=exec_script_path,
                            remote_path=f"fl_client_exec_{i}.py"
                        )
                        
                        # Execute FL client
                        script_config = ScriptConfig(
                            env=env_name,
                            script_path=f"fl_client_exec_{i}.py"
                        )
                        
                        result = client.compute.execute_script(device.id, script_config)
                        client_results.append((device.id, result))
                        
                        if result.success:
                            print(f"    ✓ FL client {i} completed successfully")
                            print("    Output preview:")
                            output_lines = result.result.split('\\n')
                            for line in output_lines[-5:]:  # Show last 5 lines
                                if line.strip():
                                    print(f"      {line}")
                        else:
                            print(f"    ✗ FL client {i} failed")
                            if result.error:
                                print(f"      Error: {result.error}")
                                
                    finally:
                        os.unlink(exec_script_path)
                        
                except ComputeError as e:
                    print(f"    ✗ Failed to start FL client {i}: {e}")
                    client_results.append((device.id, None))
                    
                print()
            
            # 5. Collect and summarize results
            print("5. Federated Learning Results Summary:")
            print("=" * 40)
            
            successful_clients = sum(1 for _, result in client_results if result and result.success)
            total_clients = len(client_results)
            
            print(f"Total clients: {total_clients}")
            print(f"Successful clients: {successful_clients}")
            print(f"Success rate: {successful_clients/total_clients*100:.1f}%")
            print()
            
            print("Individual client results:")
            for i, (device_id, result) in enumerate(client_results):
                status = "✓ Success" if result and result.success else "✗ Failed"
                print(f"  Client {i} ({device_id}): {status}")
                
                if result and result.success and "Final results:" in result.result:
                    # Try to extract final accuracy from output
                    try:
                        lines = result.result.split('\\n')
                        for line in lines:
                            if "final_accuracy" in line.lower():
                                print(f"    {line.strip()}")
                                break
                    except:
                        pass
            print()
            
            # 6. High-level FL workflow using built-in helpers
            print("6. Testing high-level FL API...")
            if fl_devices:
                try:
                    fl_task_config = FLTaskConfig(
                        framework="flower",
                        client_script="fl_client.py",
                        rounds=3,
                        clients_per_round=min(2, len(fl_devices))
                    )
                    
                    print(f"  Executing FL task on device: {fl_devices[0].id}")
                    fl_result = client.compute.execute_federated_task(fl_devices[0].id, fl_task_config)
                    
                    if fl_result.success:
                        print("  ✓ High-level FL task completed!")
                        print(f"    Rounds completed: {fl_result.rounds_completed}")
                        print(f"    Final metrics: {fl_result.final_metrics}")
                    else:
                        print("  ✗ High-level FL task failed")
                        if fl_result.error:
                            print(f"    Error: {fl_result.error}")
                            
                except ComputeError as e:
                    print(f"  High-level FL task failed: {e}")
            print()
            
        print("✓ Federated Learning demo completed")
        print()
        print("Note: This demo simulates FL training. In a production environment:")
        print("- FL server would run as a persistent background service")
        print("- Clients would connect to the actual server IP address")
        print("- Real datasets and models would be used")
        print("- Training would take significantly longer")
        
    except Exception as e:
        print(f"Unexpected error: {e}")
        return 1
        
    return 0


if __name__ == "__main__":
    exit(main())