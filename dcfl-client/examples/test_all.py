#!/usr/bin/env python3
"""
Real DCFL Client Testing Guide

This script provides comprehensive examples of how to test all protocols
with a real DCFL node running from stellar-go.
"""

import tempfile
import time
import logging
from pathlib import Path
import traceback
import statistics

from dcfl_client import DCFLClient
from dcfl_client.models.responses import ScriptConfig

# Configure logging to see what's happening
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)


def get_first_device(client: DCFLClient):
    return next(iter(client.list_devices()), None)

def setup_instructions():
    """Print setup instructions for running with real DCFL node."""
    print("""
🌟 DCFL Real Client Testing Setup
================================

STEP 1: Start the DCFL Node
---------------------------
In the stellar-go directory, run:

    go run cmd/stellar/main.go node

This will start the DCFL node and create a Unix socket at:
- Linux/Mac: ~/.cache/stellar/stellar.sock  
- Windows: %LOCALAPPDATA%/stellar/stellar.sock

STEP 2: Verify Node is Running
-----------------------------
Check that the socket file exists:

    ls -la ~/.cache/stellar/stellar.sock

You should see the socket file created by the node.

STEP 3: Run Tests with Real Client
---------------------------------
Execute this script to test all protocols with the real node:

    python examples/real_client_test.py

STEP 4: Test Individual Protocols
--------------------------------
Use the functions below to test specific protocols.
    """)

def test_connection():
    """Test basic connection to DCFL node."""
    print("\n=== Testing Basic Connection ===")
    
    try:
        # Test sync connection
        print("Testing connection...")
        with DCFLClient() as client:
            node_info = client.get_node_info()
            print(f"✓ Connected to node: {node_info.id}")
            print(f"  Node info: {node_info}")
            
            # List discovered devices
            devices = client.list_devices()
            print(f"✓ Found {len(devices)} devices")
            for device in devices:
                print(f"  - Device: {device.id}")
        
        return True
        
    except Exception as e:
        print(f"✗ Connection failed: {e}")
        print("\nTroubleshooting:")
        print("1. Make sure the DCFL node is running: go run cmd/stellar/main.go node")
        print("2. Check socket file exists: ls ~/.cache/stellar/stellar.sock")
        print("3. Try different socket path if using custom location")
        return False

def test_echo_protocol_real():
    """Test echo protocol with real DCFL node."""
    print("\n=== Testing Echo Protocol (Real) ===")
    
    try:
        with DCFLClient() as client:
            # Get list of devices
            devices = client.list_devices()
            if not devices:
                print("⚠ No devices found. Start more DCFL nodes to test inter-device communication.")
                return
            
            # Test ping to each device
            echo = client.echo
            for device in devices[:3]:  # Test first 3 devices
                try:
                    result = echo.ping(device.id)
                    print(f"✓ Ping {device.id}: {result.status}")
                except Exception as e:
                    print(f"✗ Ping {device.id} failed: {e}")
        
    except Exception as e:
        print(f"✗ Echo protocol test failed: {e}")

def test_file_protocol_real():
    """Test file protocol with real DCFL node."""
    print("\n=== Testing File Protocol (Real) ===")
    
    try:
        with DCFLClient() as client:
            devices = client.list_devices()
            if not devices:
                print("⚠ No devices found for file testing.")
                return
            
            file_protocol = client.file
            target_device = get_first_device(client).id
            
            try:
                # List files on remote device
                files = file_protocol.list_files(target_device, "")
                print(f"✓ Listed {len(files)} files on {target_device}")
                
                # Test upload (create a test file first)
                with tempfile.TemporaryDirectory() as tmpdirname:
                    test_file = Path(tmpdirname) / "dcfl_test.txt"
                    
                    with open(test_file, "w") as f:
                        f.write("Hello from DCFL client!")
                    f.close()
                    
                    success = file_protocol.upload_file(
                        target_device, 
                        str(test_file), 
                        "dcfl_uploaded.txt"
                    )
                    print(f"✓ Upload test file: {'Success' if success else 'Failed'}")
                    
                    # Test download
                    success = file_protocol.download_file(
                        target_device,
                        "/tmp/dcfl_uploaded.txt",
                        "/tmp/dcfl_downloaded.txt"
                    )
                    print(f"✓ Download test file: {'Success' if success else 'Failed'}")
                
                Path("/tmp/dcfl_downloaded.txt").unlink(missing_ok=True)
                
            except Exception as e:
                print(f"✗ File operation failed: {e}")
        
    except Exception as e:
        print(f"✗ File protocol test failed: {e}")

def test_compute_protocol_real():
    """Test compute protocol with real DCFL node."""
    print("\n=== Testing Compute Protocol (Real) ===")
    
    try:
        with DCFLClient() as client:
            devices = client.list_devices()
            if not devices:
                print("⚠ No devices found for compute testing.")
                return
            
            compute = client.compute
            target_device = get_first_device(client).id
            
            try:
                # List conda environments
                envs = compute.list_conda_envs(target_device)
                print(f"✓ Found {len(envs)} conda environments on {target_device}")
                for name, path in envs.items():
                    print(f"  - {name}: {path}")
                
                # Test script execution
                script_config = ScriptConfig(
                    script_path="/tmp/test_script.py",
                    env="base",
                    timeout=30
                )
                
                # Create a simple test script
                test_script = Path("/tmp/test_script.py")
                test_script.write_text("""
import sys
print("Hello from remote execution!")
print(f"Python version: {sys.version}")
print("Script completed successfully")
""")
                
                result = compute.execute_script(target_device, script_config)
                print(f"✓ Script execution: {'Success' if result.success else 'Failed'}")
                if result.success:
                    print(f"  Output: {result.output[:100]}...")
                else:
                    print(f"  Error: {result.error}")
                
                # Cleanup
                test_script.unlink(missing_ok=True)
                
            except Exception as e:
                print(f"✗ Compute operation failed: {e}")
        
    except Exception as e:
        print(f"✗ Compute protocol test failed: {e}")

def test_proxy_protocol_real():
    """Test proxy protocol with real DCFL node.""" 
    print("\n=== Testing Proxy Protocol (Real) ===")
    
    try:
        with DCFLClient() as client:
            devices = client.list_devices()
            if not devices:
                print("⚠ No devices found for proxy testing.")
                return
            
            proxy = client.proxy
            target_device = get_first_device(client).id
            
            try:
                # Test TCP proxy creation
                proxy_info = proxy.create_tcp_proxy(
                    target_device,
                    local_port=8080,
                    remote_host="httpbin.org",
                    remote_port=80
                )
                print(f"✓ Created TCP proxy: {proxy_info.remote_addr} on port {proxy_info.local_port}")
                
                # Test HTTP proxy creation
                http_proxy = proxy.create_http_proxy(
                    target_device,
                    local_port=8081,
                    remote_url="http://httpbin.org"
                )
                assert http_proxy
                print(f"✓ Created HTTP proxy: {http_proxy.remote_addr} on port {http_proxy.local_port}")
                
                # List active proxies
                proxies = proxy.list_proxies()
                print(f"✓ Active proxies: {len(proxies)}")
                
                # Test proxy connection
                test_result = proxy.test_http_proxy_connection(8080)
                print(f"✓ Proxy test: {'Success' if test_result else 'Failed'}")
                
                # Cleanup - close proxies
                closed = proxy.close_all_proxies()
                print(f"✓ Closed {closed} proxies")
                
            except Exception as e:
                print(f"✗ Proxy operation failed: {e}")
                traceback.print_exc()
        
    except Exception as e:
        print(f"✗ Proxy protocol test failed: {e}")

def test_advanced_features():
    """Test advanced async features with real node."""
    print("\n=== Testing Advanced Features ===")
    
    try:
        def advanced_test():
            with DCFLClient() as client:
                # Test operations across multiple protocols
                echo = client.echo
                file_proto = client.file
                compute = client.compute
                
                devices = client.list_devices()
                if len(devices) < 1:
                    print("⚠ Need at least 2 devices for advanced testing")
                    return
                
                # Ping all devices
                start_time = time.time()
                successful_pings = 0
                for device in devices:
                    _ = echo.ping(device.id)
                    successful_pings += 1
                ping_time = time.time() - start_time
                
                print(f"✓ Sequential ping: {successful_pings}/{len(devices)} in {ping_time:.2f}s")
                
                # Test batch operations
                if successful_pings > 0:
                    # File operations on multiple devices
                    for device in devices[:3]:  # First 3 devices
                        try:
                            files = file_proto.list_files(device.id, "/")
                            print(f"✓ Listed {len(files)} files on {device.id}")
                        except Exception as e:
                            print(f"  File listing failed on {device.id}: {e}")
                    
                    print(f"✓ Advanced operations completed")
        
        advanced_test()
        
    except Exception as e:
        print(f"✗ Advanced features test failed: {e}")

def performance_benchmark():
    """Run performance benchmarks with real node."""
    print("\n=== Performance Benchmark ===")
    
    try:
        with DCFLClient() as client:
            devices = client.list_devices()
            if not devices:
                print("⚠ No devices for performance testing")
                return
            
            echo = client.echo
            
            # Benchmark sync vs async performance
            device_id = get_first_device(client).id
            
            # Sync benchmark
            sync_times = []
            for _ in range(5):
                start = time.time()
                _ = echo.ping(device_id)
                sync_times.append(time.time() - start)
            
            # Second sync benchmark
            sync_times2 = []
            for _ in range(5):
                start = time.time()
                _ = echo.ping(device_id)
                sync_times2.append(time.time() - start)
            
            if sync_times and sync_times2:
                avg_sync = statistics.mean(sync_times) * 1000  # Convert to ms
                avg_sync2 = statistics.mean(sync_times2) * 1000
                
                print(f"✓ First sync ping average: {avg_sync:.1f}ms")
                print(f"✓ Second sync ping average: {avg_sync2:.1f}ms")
                print(f"  Performance difference: {abs(avg_sync - avg_sync2):.1f}ms")
        
    except Exception as e:
        print(f"✗ Performance benchmark failed: {e}")

def main():
    """Main testing function."""
    print("🚀 DCFL Real Client Testing Suite")
    print("=" * 50)
    
    # # Show setup instructions first
    # setup_instructions()
    
    # Test basic connection first
    if not test_connection():
        print("\n❌ Basic connection failed. Please check setup instructions above.")
        return
    
    # Run all protocol tests
    test_echo_protocol_real()
    test_file_protocol_real()
    # test_compute_protocol_real()
    test_proxy_protocol_real()
    
    # Advanced features
    test_advanced_features()
    
    # Performance benchmark
    performance_benchmark()
    
    print("\n" + "=" * 50)
    print("✅ Real Client Testing Complete!")

if __name__ == "__main__":
    main()