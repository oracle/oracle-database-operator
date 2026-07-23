import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.NoSuchFileException;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.security.KeyFactory;
import java.security.PrivateKey;
import java.security.spec.PKCS8EncodedKeySpec;
import java.security.spec.MGF1ParameterSpec;
import javax.crypto.Cipher;
import javax.crypto.spec.OAEPParameterSpec;
import javax.crypto.spec.PSource;
import java.util.Base64;
import java.util.regex.Pattern;

public class RSADecryptOAEP {

    private static final Path PRIVATE_KEY_DIR =
            Paths.get("/opt/oracle/sa/encryptionPrivateKey").toAbsolutePath().normalize();
    private static final Pattern KEY_NAME_PATTERN = Pattern.compile("[A-Za-z0-9._-]+");
    private static final OAEPParameterSpec OAEP_PARAMS = new OAEPParameterSpec(
            "SHA-256",
            "MGF1",
            new MGF1ParameterSpec("SHA-256"),
            PSource.PSpecified.DEFAULT
    );

    public static void main(String[] args) {
        if (args == null || args.length != 2) {
            System.err.println("Usage: RSADecryptOAEP <privateKeyFilename> <base64EncryptedValue>");
            System.exit(2);
        }

        String privateKeyName = args[0];
        String encodedEncryptedPassword = args[1];

        try {
            Path keyPath = resolveKeyPath(privateKeyName);

            String pem = Files.readString(keyPath, StandardCharsets.UTF_8);
            String enc64 = pem
                    .replace("-----BEGIN PRIVATE KEY-----", "")
                    .replace("-----END PRIVATE KEY-----", "")
                    .replaceAll("\\s", "");

            byte[] privateKeyBytes = Base64.getDecoder().decode(enc64);
            KeyFactory keyFactory = KeyFactory.getInstance("RSA");
            PKCS8EncodedKeySpec keySpec = new PKCS8EncodedKeySpec(privateKeyBytes);
            PrivateKey privateKey = keyFactory.generatePrivate(keySpec);

            byte[] encryptedData = Base64.getMimeDecoder()
                    .decode(encodedEncryptedPassword.getBytes(StandardCharsets.UTF_8));

            Cipher cipher = Cipher.getInstance("RSA/ECB/OAEPWithSHA-256AndMGF1Padding");
            cipher.init(Cipher.DECRYPT_MODE, privateKey, OAEP_PARAMS);

            byte[] decryptedData = cipher.doFinal(encryptedData);
            System.out.print(new String(decryptedData, StandardCharsets.UTF_8));
        } catch (SecurityException e) {
            System.err.println("Error: Invalid key name or path.");
            System.exit(3);
        } catch (NoSuchFileException e) {
            System.err.println("Error: Private key file not found or not readable.");
            System.exit(4);
        } catch (IllegalArgumentException e) {
            System.err.println("Error: Invalid input format.");
            System.exit(5);
        } catch (IOException e) {
            System.err.println("Error: I/O failure accessing key or inputs.");
            System.exit(6);
        } catch (Exception e) {
            System.err.println("Error: Decryption failed.");
            System.exit(7);
        }
    }

    private static Path resolveKeyPath(String privateKeyName) throws NoSuchFileException {
        if (!KEY_NAME_PATTERN.matcher(privateKeyName).matches()) {
            throw new SecurityException("Error: Invalid key filename.");
        }

        Path requestedPath = PRIVATE_KEY_DIR.resolve(privateKeyName).normalize();
        if (!requestedPath.startsWith(PRIVATE_KEY_DIR)) {
            throw new SecurityException("Error: Invalid key path.");
        }

        if (!Files.isRegularFile(requestedPath) || !Files.isReadable(requestedPath)) {
            throw new NoSuchFileException(privateKeyName);
        }

        return requestedPath;
    }
}
