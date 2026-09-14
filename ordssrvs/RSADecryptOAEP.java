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
import java.util.stream.Collectors;

public class RSADecryptOAEP {

    private static final Path PRIVATE_KEY_DIR =
            Paths.get("/opt/oracle/sa/encryptionPrivateKey").toAbsolutePath().normalize();
    private static final OAEPParameterSpec OAEP_PARAMS = new OAEPParameterSpec(
            "SHA-256",
            "MGF1",
            new MGF1ParameterSpec("SHA-256"),
            PSource.PSpecified.DEFAULT
    );

    public static void main(String[] args) {
        if (args == null || args.length != 2) {
            System.err.println("Decrypt Failed");
            System.exit(1);
        }

        String privateKeyName = args[0];
        String encodedEncryptedPassword = args[1];

        try {
            if (!privateKeyName.matches("[A-Za-z0-9._-]+")) {
                throw new SecurityException("Invalid key filename.");
            }

            Path requestedPath = PRIVATE_KEY_DIR.resolve(privateKeyName).normalize();
            if (!requestedPath.startsWith(PRIVATE_KEY_DIR)) {
                throw new SecurityException("Invalid key path.");
            }

            if (!Files.isRegularFile(requestedPath) || !Files.isReadable(requestedPath)) {
                throw new NoSuchFileException("File not found.");
            }

            String pem = Files.readString(requestedPath, StandardCharsets.UTF_8);
            String pemWithoutBoundaries = pem.lines()
                    .filter(line -> !line.startsWith("-----"))
                    .collect(Collectors.joining("\n"));
            String enc64 = pemWithoutBoundaries.replaceAll("\\s", "");

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
        } catch (Exception e) {
            System.err.println("Decrypt Failed");
            System.exit(1);
        }
    }
}
