import subprocess
import json
import os

'''
Provisions a Yubikey and writes public keys and certs to the appropriate folder.

Note: You must have cosign installed and replace the COSIGN path below.
'''

COSIGN="/home/asraa/git/cosign/cmd/cosign/cosign"
KEY_DIR = 'ceremony/2021-05-03/ceremony-products'

class HSM(object):
    ''' HSM provisioned key object '''
    def __init__(self):
        subprocess.run([COSIGN, "piv-tool", "reset"])
        # TODO: I only want the prompts, not the attestation outputs.
        # TODO: Why does this create repeated prompts?
        subprocess.run([COSIGN, "piv-tool", "generate-key", "--random-management-key"])
        output = subprocess.Popen([COSIGN, "piv-tool", "attestation", "-output", "json"], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        stdout, stderr = output.communicate()
        if stdout == None:
            raise 
        key = json.loads(str(stdout))
        self.serial = str(key['KeyAttestation']['Serial'])
        self.device_cert = key['DeviceCertPem']
        self.key_cert = key['KeyCertPem']
        output = subprocess.Popen([COSIGN, "public-key", "-sk"], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        stdout, stderr = output.communicate()
        self.public_key = str(stdout)

    def write(self):
        # Writes files
        directory = os.path.join(KEY_DIR, self.serial)
        try: 
            os.mkdir(directory)
        except:
            print("\nDirectory already exists for key. Manually remove stale directory")
            return
        print("Created directory %s" % directory)
        files = {"_pubkey.pem": self.public_key, "_device_cert.pem": self.device_cert, "_key_cert.pem": self.key_cert}
        for key, value in files.items():
            filename = os.path.join(directory, self.serial + key)
            f = open(filename, "x")
            f.write(value)



def main():
    # TODO: Maybe add flag in case someone wants to just rewrite files.
    # TODO: Add error handling in case cosign execute isn't there.
    yubikey = HSM()
    yubikey.write()

if __name__ == "__main__":
    main()


