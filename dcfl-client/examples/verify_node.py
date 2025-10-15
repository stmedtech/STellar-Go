#!/usr/bin/env python3
"""
DCFL Node Verification Script

Quick script to verify if the DCFL node is running and accessible.
"""

import os
import sys
from pathlib import Path
import traceback

def test_basic_connection():
    """Test basic connection to DCFL node."""
    try:
        from dcfl_client import DCFLClient
        
        print(f"\n🔌 Testing connection...")
        
        # Try sync connection
        try:
            # with DCFLClient(socket_path=socket_path) as client:
            with DCFLClient() as client:
                node_info = client.get_node_info()
                print(f"✅ Successfully connected!")
                print(f"   Node ID: {node_info.id}")
                print(f"   Node info: {node_info}")
                
                # Test device discovery
                devices = client.list_devices()
                print(f"   Discovered devices: {len(devices)}")
                
                return True
                
        except Exception as sync_error:
            print(f"❌ Sync connection failed: {sync_error}")
            traceback.print_exc()
            return False
            
    except ImportError:
        print("❌ dcfl_client not installed. Run: pip install -e .")
        return False

def check_node_process():
    """Check if stellar node process is running."""
    import subprocess
    
    print("\n🔍 Checking for running stellar processes...")
    
    try:
        # Try to find stellar processes
        result = subprocess.run(
            ["ps", "aux"],
            capture_output=True,
            text=True,
            timeout=10
        )
        
        stellar_processes = [
            line for line in result.stdout.split('\n')
            if 'stellar' in line.lower() and 'node' in line
        ]
        
        if stellar_processes:
            print("✅ Found stellar processes:")
            for process in stellar_processes:
                print(f"   {process.strip()}")
            return True
        else:
            print("❌ No stellar node processes found")
            return False
            
    except subprocess.TimeoutExpired:
        print("⚠ Process check timed out")
        return False
    except FileNotFoundError:
        print("⚠ 'ps' command not available (Windows?)")
        # On Windows, try tasklist instead
        try:
            result = subprocess.run(
                ["tasklist", "/fi", "IMAGENAME eq stellar*"],
                capture_output=True,
                text=True,
                timeout=10
            )
            if "stellar" in result.stdout.lower():
                print("✅ Found stellar process on Windows")
                return True
            else:
                print("❌ No stellar processes found on Windows")
                return False
        except:
            print("❌ Could not check processes on Windows")
            return False

def provide_setup_help():
    """Provide help on setting up the DCFL node."""
    print("""
📋 DCFL Node Setup Instructions
===============================

STEP 1: Build and Run the Node
------------------------------
In the stellar-go directory:

    go run cmd/stellar/main.go node

Or build and run:

    go build -o stellar cmd/stellar/main.go
    ./stellar node

STEP 2: Verify Node Started
---------------------------
You should see output like:

    Starting DCFL node...
    Socket created at: ~/.cache/stellar/stellar.sock
    DHT bootstrap successful
    Node ID: QmXXXXXXX...

STEP 3: Test Connection
----------------------
Run this script again:

    python examples/verify_node.py

TROUBLESHOOTING
--------------
• Socket permission issues: chmod 666 ~/.cache/stellar/stellar.sock
• Port conflicts: Use --port flag: go run cmd/stellar/main.go node --port 4002
• Firewall blocking: Check firewall settings for libp2p ports
• Network discovery: Wait 30-60 seconds for peer discovery
""")

def main():
    """Main verification function."""
    print("🌟 DCFL Node Verification")
    print("=" * 40)
    
    # Check if processes are running
    process_running = check_node_process()
    
    # Test connection
    connection_ok = test_basic_connection()
    
    if process_running:
        if connection_ok:
            print("\n🎉 SUCCESS!")
            print("✅ DCFL node is running and accessible")
            print("✅ Ready for real client testing")
            return True
        else:
            print("\n⚠ PARTIAL SUCCESS")
            print("• Socket file exists but connection failed")
            print("• Node may be starting up or misconfigured")
    else:
        print("\n❌ NODE NOT RUNNING")
        provide_setup_help()
        return False
    
    return False

if __name__ == "__main__":
    success = main()
    sys.exit(0 if success else 1)